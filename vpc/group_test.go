package vpc_test

import (
	"os"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }

func TestVpcs(t *testing.T)           { engine.TestGroup(t, "vpc", "vpcs") }
func TestSubnets(t *testing.T)        { engine.TestGroup(t, "vpc", "subnets") }
func TestSecurityGroups(t *testing.T) { engine.TestGroup(t, "vpc", "securitygroups") }
func TestGaps(t *testing.T)           { engine.TestGroup(t, "vpc", "gaps") }
