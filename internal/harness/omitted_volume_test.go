package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOmittedLedgerVolumeKeepsTheDeleteError(t *testing.T) {
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		switch action {
		case "DetachVolume":
			writeEC2(w, 400, ec2Err("InvalidAction", "DetachVolume"))
		case "DeleteVolume":
			deletes.Add(1)
			writeEC2(w, 400, ec2Err("IncorrectState", "volume is in use"))
		case "DescribeVolumes":
			writeEC2(w, 200, ec2OK("DescribeVolumesResponse", "<volumeSet/>"))
		default:
			status, body := ec2Describe(action, false)
			writeEC2(w, status, body)
		}
	}))
	defer srv.Close()
	rt := runtimeOn(t, srv.URL, "vol-omitted")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := rt.Sweep(ctx)
	if err == nil || !strings.Contains(err.Error(), "IncorrectState") || !strings.Contains(err.Error(), "vol-omitted") {
		t.Fatalf("deletes %d err %v", deletes.Load(), err)
	}
}
