package harness

import (
	"context"
	"strings"
	"testing"
)

func TestLoadRefusesAmazonBeforeClientUse(t *testing.T) {
	cfg := Config{
		AccessKey:       "AKIDEXAMPLE",
		Secret:          "secret",
		Region:          "vws-1",
		EC2Endpoint:     "https://ec2.us-east-1.amazonaws.com",
		AllowedHosts:    []string{"ec2.us-east-1.amazonaws.com"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake",
	}
	_, err := Load(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("got %v", err)
	}
}
