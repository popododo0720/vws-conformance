package cases

const (
	KindHappy       = "happy"
	KindError       = "error"
	KindPagination  = "pagination"
	KindIdempotency = "idempotency"
	KindTags        = "tags"
	KindFilter      = "filter"
	KindAuth        = "auth"
	KindGap         = "gap"

	SuiteSDK = "go-sdk"
	SuiteCLI = "cli"
	SuiteRaw = "raw-http"

	SkipIAM     = "skipped until vws-deploy#310 lands and console-api is wired to vws-iam"
	SkipMFAAuth = "skipped until a caller without iam:GetMFADevice is available; " +
		"the suite runs one credential (popododo0720/vws-deploy#310)"
	SkipUnsafeFixedID = "skipped until an owned gone id exists; " +
		"a fixed id can change a foreign resource (popododo0720/vws-conformance#1)"
)

type Expect struct {
	HTTP     int
	Code     string
	Patterns map[string]string
	Head     bool
}

type Case struct {
	ID      string
	Service string
	Group   string
	Op      string
	Kind    string
	Suite   string
	DocURL  string
	Expect  Expect
	Skip    string
	Slow    bool
	Impl    string
}

type Outcome struct {
	HTTP    int
	Code    string
	Message string
	Fields  map[string]string
	Raw     string
}

func DocEC2(op string) string {
	return "https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_" + op + ".html"
}

func DocS3(op string) string {
	return "https://docs.aws.amazon.com/AmazonS3/latest/API/API_" + op + ".html"
}

func DocIAM(op string) string {
	return "https://docs.aws.amazon.com/IAM/latest/APIReference/API_" + op + ".html"
}

func DocSTS(op string) string {
	return "https://docs.aws.amazon.com/STS/latest/APIReference/API_" + op + ".html"
}

const (
	DocS3Errors  = "https://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html"
	DocEC2Errors = "https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html"
	DocIAMErrors = "https://docs.aws.amazon.com/IAM/latest/APIReference/CommonErrors.html"
	DocSigV4     = "https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv.html"
)
