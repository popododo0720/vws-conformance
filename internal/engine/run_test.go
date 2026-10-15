package engine

import "testing"

func TestLoadConfigRejectsForeignRunID(t *testing.T) {
	t.Setenv("VWS_CONF_RUN_ID", "prod-bucket")
	_, err := loadConfig()
	if err == nil {
		t.Fatal("accepted prod-bucket")
	}
}
