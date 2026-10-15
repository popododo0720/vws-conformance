package engine

import "testing"

func TestLoadConfigRejectsForeignRunID(t *testing.T) {
	t.Setenv("VWS_CONF_RUN_ID", "prod-bucket")
	_, err := loadConfig()
	if err == nil {
		t.Fatal("accepted prod-bucket")
	}
}

func TestLoadConfigReusesRunID(t *testing.T) {
	t.Setenv("VWS_CONF_RUN_ID", "")
	first, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID == "" || first.RunID != second.RunID {
		t.Fatalf("%s vs %s", first.RunID, second.RunID)
	}
}

func TestLoadConfigKeepsExplicitRunID(t *testing.T) {
	const id = "conf-20261015T120000Z-abc123"
	t.Setenv("VWS_CONF_RUN_ID", id)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunID != id {
		t.Fatal(cfg.RunID)
	}
}
