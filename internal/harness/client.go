package harness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

type captureKey struct{}

type Capture struct {
	mu      sync.Mutex
	Status  int
	Header  http.Header
	Body    []byte
	Method  string
	URL     string
	ReqHead http.Header
}

func (c *Capture) snapshot(secret string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s %s\n", c.Method, c.URL)
	for k, vs := range RedactHeader(c.ReqHead) {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	fmt.Fprintf(&b, "\nHTTP %d\n", c.Status)
	for k, vs := range RedactHeader(c.Header) {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	b.Write(c.Body)
	return Redact(b.String(), secret)
}

func withCapture(ctx context.Context, c *Capture) context.Context {
	return context.WithValue(ctx, captureKey{}, c)
}

func captureFrom(ctx context.Context) *Capture {
	c, _ := ctx.Value(captureKey{}).(*Capture)
	return c
}

func captureOption() func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		if err := stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("confCaptureReq", func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
			if c := captureFrom(ctx); c != nil {
				if req, ok := in.Request.(*smithyhttp.Request); ok && req.Request != nil {
					c.mu.Lock()
					c.Method = req.Method
					c.URL = req.URL.String()
					c.ReqHead = req.Header.Clone()
					c.mu.Unlock()
				}
			}
			return next.HandleFinalize(ctx, in)
		}), middleware.Before); err != nil {
			return err
		}
		return stack.Deserialize.Add(middleware.DeserializeMiddlewareFunc("confCaptureResp", func(ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler) (middleware.DeserializeOutput, middleware.Metadata, error) {
			out, md, err := next.HandleDeserialize(ctx, in)
			if c := captureFrom(ctx); c != nil {
				if resp, ok := out.RawResponse.(*smithyhttp.Response); ok && resp != nil && resp.Response != nil {
					body, _ := io.ReadAll(resp.Body)
					resp.Body = io.NopCloser(bytes.NewReader(body))
					c.mu.Lock()
					c.Status = resp.StatusCode
					c.Header = resp.Header.Clone()
					c.Body = body
					c.mu.Unlock()
				}
			}
			return out, md, err
		}), middleware.After)
	}
}

type Clients struct {
	EC2 *ec2.Client
	S3  *s3.Client
	IAM *iam.Client
	STS *sts.Client
	Raw *http.Client
}

func Load(ctx context.Context, cfg Config) (*Clients, error) {
	if err := cfg.CheckEndpoints(); err != nil {
		return nil, err
	}
	if cfg.AccessKey == "" || cfg.Secret == "" {
		return nil, fmt.Errorf("VWS_CONF_ACCESS_KEY_ID and VWS_CONF_SECRET_ACCESS_KEY are required")
	}
	if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_DEFAULT_PROFILE") != "" {
		return nil, fmt.Errorf("AWS_PROFILE is refused; set VWS_CONF_ACCESS_KEY_ID and VWS_CONF_SECRET_ACCESS_KEY")
	}
	_ = os.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	httpClient := NewHTTPClient(cfg.AllowedHosts, cfg.AllowedCIDRs)
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.Secret, cfg.Session)),
		config.WithEC2IMDSClientEnableState(imds.ClientDisabled),
		config.WithHTTPClient(httpClient),
		config.WithSharedConfigFiles([]string{}),
		config.WithSharedCredentialsFiles([]string{}),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 2 })
		}),
		config.WithAPIOptions([]func(*middleware.Stack) error{captureOption()}),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	cl := &Clients{Raw: httpClient}
	if cfg.EC2Endpoint != "" {
		cl.EC2 = ec2.NewFromConfig(awsCfg, func(o *ec2.Options) {
			o.BaseEndpoint = aws.String(cfg.EC2Endpoint)
		})
	}
	if cfg.S3Endpoint != "" {
		cl.S3 = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		})
	}
	if cfg.IAMEndpoint != "" {
		cl.IAM = iam.NewFromConfig(awsCfg, func(o *iam.Options) {
			o.BaseEndpoint = aws.String(cfg.IAMEndpoint)
		})
		cl.STS = sts.NewFromConfig(awsCfg, func(o *sts.Options) {
			o.BaseEndpoint = aws.String(cfg.IAMEndpoint)
		})
	}
	return cl, nil
}

type Runtime struct {
	Cfg       Config
	Clients   *Clients
	Clean     Cleaner
	Dry       bool
	secret    string
	netMu     sync.Mutex
	netFlight *netFlight
	net       netIDs
}

func NewRuntime(cfg Config, cl *Clients) *Runtime {
	return &Runtime{Cfg: cfg, Clients: cl, secret: cfg.Secret}
}

func (rt *Runtime) RunID() string { return rt.Cfg.RunID }

func (rt *Runtime) Region() string { return rt.Cfg.Region }

func (rt *Runtime) Name(suffix string) string { return ResourceName(rt.Cfg.RunID, suffix) }

func (rt *Runtime) Cleanup(fn func(context.Context) error) { rt.Clean.Add(fn) }

func (rt *Runtime) Call(ctx context.Context, fn func(context.Context) error) (cases.Outcome, error) {
	cap := &Capture{}
	ctx = withCapture(ctx, cap)
	ctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	err := fn(ctx)
	o := cases.Outcome{Fields: map[string]string{}, HTTP: cap.Status, Raw: cap.snapshot(rt.secret)}
	if err == nil {
		if o.HTTP == 0 {
			return o, fmt.Errorf("response status was not captured")
		}
		return o, nil
	}
	var ae smithy.APIError
	if !errors.As(err, &ae) {
		return o, fmt.Errorf("call: %w", err)
	}
	o.Code = ae.ErrorCode()
	o.Message = ae.ErrorMessage()
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.Response != nil {
		o.HTTP = re.HTTPStatusCode()
	}
	return o, nil
}

func UnsupportedParameter(err error) bool {
	var ae smithy.APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.ErrorCode() == "InvalidParameterValue" && ae.ErrorMessage() == "unsupported"
}

func PayloadHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
