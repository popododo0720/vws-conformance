package engine

import "testing"

func TestCheckCLIRefusesAmazon(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("VWS_CONF_RUN_ID", "")
	t.Setenv("VWS_CONF_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("VWS_CONF_SECRET_ACCESS_KEY", "secret")
	t.Setenv("VWS_CONF_S3_ENDPOINT", "https://s3.us-east-1.amazonaws.com")
	t.Setenv("VWS_CONF_ALLOWED_HOSTS", "s3.us-east-1.amazonaws.com")
	t.Setenv("VWS_CONF_TARGET", "local-fake")
	if CheckCLI() == 0 {
		t.Fatal("accepted amazonaws")
	}
}

func TestCheckCLIAcceptsLoopback(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("VWS_CONF_RUN_ID", "")
	t.Setenv("VWS_CONF_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("VWS_CONF_SECRET_ACCESS_KEY", "secret")
	t.Setenv("VWS_CONF_S3_ENDPOINT", "http://127.0.0.1:8082")
	t.Setenv("VWS_CONF_ALLOWED_HOSTS", "127.0.0.1")
	t.Setenv("VWS_CONF_TARGET", "local-fake")
	if CheckCLI() != 0 {
		t.Fatal("refused loopback")
	}
}
