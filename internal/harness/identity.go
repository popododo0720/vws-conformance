package harness

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func (rt *Runtime) Identify(ctx context.Context) (string, error) {
	if rt.Clients == nil || rt.Clients.STS == nil {
		return "", fmt.Errorf("IAM endpoint is unset")
	}
	out, err := withDeadline(ctx, func(ctx context.Context) (*sts.GetCallerIdentityOutput, error) {
		return rt.Clients.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	})
	if err != nil {
		return "", fmt.Errorf("GetCallerIdentity: %w", err)
	}
	return aws.ToString(out.Account), nil
}

func (rt *Runtime) BindAccount(account string) error {
	if account == "" {
		return fmt.Errorf("verified account is empty")
	}
	if rt.Cfg.AccountID != "" && rt.Cfg.AccountID != account {
		return fmt.Errorf("configured account %s does not match verified account %s", rt.Cfg.AccountID, account)
	}
	rt.Cfg.AccountID = account
	return nil
}
