package harness

import "testing"

func TestFromEnvRegion(t *testing.T) {
	t.Setenv("VWS_CONF_REGION", "")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Region != "vws-1" {
		t.Fatalf("default region %q", c.Region)
	}
	t.Setenv("VWS_CONF_REGION", "pve-test-1")
	c, err = FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Region != "pve-test-1" {
		t.Fatalf("region %q", c.Region)
	}
}

func TestFromEnvTarget(t *testing.T) {
	for target, ok := range map[string]bool{
		"vws-test": true, "local-fake": true, "local-pve": true, "pve-test": true,
		"local-grpc": false, "": true, "aws": false,
	} {
		t.Setenv("VWS_CONF_TARGET", target)
		_, err := FromEnv()
		if (err == nil) != ok {
			t.Errorf("target %q: err=%v, want ok=%v", target, err, ok)
		}
	}
}
