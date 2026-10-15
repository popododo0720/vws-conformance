package harness

import "testing"

func TestRejectsAmazonAWSEvenIfListed(t *testing.T) {
	err := CheckEndpoint("https://s3.us-east-1.amazonaws.com", []string{"s3.us-east-1.amazonaws.com"})
	if err == nil {
		t.Fatal("expected reject")
	}
	if err.Error() != `endpoint host "s3.us-east-1.amazonaws.com" is refused` {
		t.Fatalf("%v", err)
	}
	err = CheckEndpoint("https://amazonaws.com/bucket", []string{"amazonaws.com"})
	if err == nil || err.Error() != `endpoint host "amazonaws.com" is refused` {
		t.Fatalf("%v", err)
	}
}

func TestAllowLoopback(t *testing.T) {
	if err := CheckEndpoint("http://127.0.0.1:8082", []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnlistedHost(t *testing.T) {
	err := CheckEndpoint("http://127.0.0.1:8082", []string{"10.77.0.241"})
	if err == nil || err.Error() != `endpoint host "127.0.0.1" is not in the allow list` {
		t.Fatalf("%v", err)
	}
}

func TestAccountGuard(t *testing.T) {
	if err := CheckAccount("123456789012", []string{"123456789012"}); err != nil {
		t.Fatal(err)
	}
	err := CheckAccount("999999999999", []string{"123456789012"})
	if err == nil || err.Error() != "account 999999999999 is not allowed" {
		t.Fatalf("%v", err)
	}
}
