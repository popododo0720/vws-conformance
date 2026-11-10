package iam_test

import (
	"os"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}

func TestUsers(t *testing.T)      { engine.TestGroup(t, "iam", "users") }
func TestMFA(t *testing.T)        { engine.TestGroup(t, "iam", "mfa") }
func TestMFATags(t *testing.T)    { engine.TestGroup(t, "iam", "mfatags") }
func TestPolicyTags(t *testing.T) { engine.TestGroup(t, "iam", "policytags") }
func TestRoles(t *testing.T)      { engine.TestGroup(t, "iam", "roles") }
func TestMFADevices(t *testing.T) { engine.TestGroup(t, "iam", "mfadevices") }
