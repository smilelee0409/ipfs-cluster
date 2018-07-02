package stateless

import (
	"context"
	"errors"
	"flag"
	"fmt"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	"sort"
	"strings"
	"testing"
	"time"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

var (
	logLevel               = "CRITICAL"
	customLogLvlFacilities = logFacilities{}
	pinCancelCid           = test.TestCid3
	unpinCancelCid         = test.TestCid2
	ErrPinCancelCid        = errors.New("should not have received rpc.IPFSPin operation")
	ErrUnpinCancelCid      = errors.New("should not have received rpc.IPFSUnpin operation")
)

type logFacilities []string

// String is the method to format the flag's value, part of the flag.Value interface.
func (lg *logFacilities) String() string {
	return fmt.Sprint(*lg)
}

// Set is the method to set the flag value, part of the flag.Value interface.
func (lg *logFacilities) Set(value string) error {
	if len(*lg) > 0 {
		return errors.New("logFacilities flag already set")
	}
	for _, lf := range strings.Split(value, ",") {
		*lg = append(*lg, lf)
	}
	return nil
}

func init() {
	flag.Var(&customLogLvlFacilities, "logfacs", "use -logLevel for only the following log facilities; comma-separated")
	flag.StringVar(&logLevel, "loglevel", logLevel, "default log level for tests")
	flag.Parse()

	if len(customLogLvlFacilities) <= 0 {
		for f := range ipfscluster.LoggingFacilities {
			ipfscluster.SetFacilityLogLevel(f, logLevel)
		}

		for f := range ipfscluster.LoggingFacilitiesExtra {
			ipfscluster.SetFacilityLogLevel(f, logLevel)
		}
	}

	for _, f := range customLogLvlFacilities {
		if _, ok := ipfscluster.LoggingFacilities[f]; ok {
			ipfscluster.SetFacilityLogLevel(f, logLevel)
			continue
		}
		if _, ok := ipfscluster.LoggingFacilitiesExtra[f]; ok {
			ipfscluster.SetFacilityLogLevel(f, logLevel)
			continue
		}
	}
}

type mockService struct {
	rpcClient *rpc.Client
}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) IPFSPin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	switch c.String() {
	case test.TestSlowCid1:
		time.Sleep(2 * time.Second)
	case pinCancelCid:
		return ErrPinCancelCid
	}
	return nil
}

func (mock *mockService) IPFSUnpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	switch c.String() {
	case test.TestSlowCid1:
		time.Sleep(2 * time.Second)
	case unpinCancelCid:
		return ErrUnpinCancelCid
	}
	return nil
}

func (mock *mockService) IPFSPinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		test.TestCid1: api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

func (mock *mockService) IPFSPinLsCid(ctx context.Context, in api.PinSerial, out *api.IPFSPinStatus) error {
	switch in.Cid {
	case test.TestCid1, test.TestCid2:
		*out = api.IPFSPinStatusRecursive
	default:
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockService) Pins(ctx context.Context, in struct{}, out *[]api.PinSerial) error {
	*out = []api.PinSerial{
		{Cid: test.TestCid1, ReplicationFactorMax: -1},
		{Cid: test.TestCid3, ReplicationFactorMax: -1},
	}
	return nil
}

func (mock *mockService) PinGet(ctx context.Context, in api.PinSerial, out *api.PinSerial) error {
	switch in.Cid {
	case test.ErrorCid:
		return errors.New("expected error when using ErrorCid")
	case test.TestCid1:
		*out = api.Pin{Cid: test.MustDecodeCid(in.Cid), ReplicationFactorMax: -1}.ToSerial()
		return nil
	case test.TestCid2:
		*out = api.Pin{Cid: test.MustDecodeCid(in.Cid), ReplicationFactorMax: -1}.ToSerial()
		return nil
	}
	*out = in
	return nil
}

func testSlowStatelessPinTracker(t *testing.T) *Tracker {
	cfg := &Config{}
	cfg.Default()
	mpt := New(cfg, test.TestPeerID1)
	mpt.SetClient(mockRPCClient(t))
	return mpt
}

func testStatelessPinTracker(t *testing.T) *Tracker {
	cfg := &Config{}
	cfg.Default()
	spt := New(cfg, test.TestPeerID1)
	spt.SetClient(test.NewMockRPCClient(t))
	return spt
}

func TestStatelessPinTracker_New(t *testing.T) {
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown()
}

func TestStatelessPinTracker_Shutdown(t *testing.T) {
	spt := testStatelessPinTracker(t)
	err := spt.Shutdown()
	if err != nil {
		t.Fatal(err)
	}
	err = spt.Shutdown()
	if err != nil {
		t.Fatal(err)
	}
}

func TestUntrackTrack(t *testing.T) {
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown()

	h1 := test.MustDecodeCid(test.TestCid1)

	// LocalPin
	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = spt.Untrack(h1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)

	// LocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let pinning start

	pInfo := spt.optracker.Get(slowPin.Cid)
	if pInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("slowPin should be tracked")
	}

	if pInfo.Status == api.TrackerStatusPinning {
		go func() {
			err = spt.Untrack(slowPinCid)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-spt.optracker.OpContext(slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be pinning and is:", pInfo.Status)
	}
}

func TestTrackUntrackWithNoCancel(t *testing.T) {
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)
	fastPinCid := test.MustDecodeCid(pinCancelCid)

	// SlowLocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	// LocalPin
	fastPin := api.Pin{
		Cid:                  fastPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Track(fastPin)
	if err != nil {
		t.Fatal(err)
	}

	// fastPin should be queued because slow pin is pinning
	fastPInfo := spt.optracker.Get(fastPin.Cid)
	if fastPInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("fastPin should be tracked")
	}
	if fastPInfo.Status == api.TrackerStatusPinQueued {
		err = spt.Untrack(fastPinCid)
		if err != nil {
			t.Fatal(err)
		}
		// pi := spt.get(fastPinCid)
		// if pi.Error == ErrPinCancelCid.Error() {
		// 	t.Fatal(ErrPinCancelCid)
		// }
	} else {
		t.Error("fastPin should be queued to pin")
	}

	pi := spt.optracker.Get(fastPin.Cid)
	if pi.Cid == nil {
		t.Error("fastPin should have been removed from tracker")
	}
}

func TestUntrackTrackWithCancel(t *testing.T) {
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)

	// LocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Untrack should cancel the ongoing request
	// and unpin right away
	err = spt.Untrack(slowPinCid)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	pi := spt.optracker.Get(slowPin.Cid)
	if pi.Cid == nil {
		t.Fatal("expected slowPin to be tracked")
	}

	if pi.Status == api.TrackerStatusUnpinning {
		go func() {
			err = spt.Track(slowPin)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-spt.optracker.OpContext(slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be in unpinning")
	}

}

func TestUntrackTrackWithNoCancel(t *testing.T) {
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)
	fastPinCid := test.MustDecodeCid(unpinCancelCid)

	// SlowLocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	// LocalPin
	fastPin := api.Pin{
		Cid:                  fastPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Track(fastPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	err = spt.Untrack(slowPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Untrack(fastPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	pi := spt.optracker.Get(fastPin.Cid)
	if pi.Cid == nil {
		t.Fatal("c untrack operation should be tracked")
	}

	if pi.Status == api.TrackerStatusUnpinQueued {
		err = spt.Track(fastPin)
		if err != nil {
			t.Fatal(err)
		}

		// pi := spt.get(fastPinCid)
		// if pi.Error == ErrUnpinCancelCid.Error() {
		// 	t.Fatal(ErrUnpinCancelCid)
		// }
	} else {
		t.Error("c should be queued to unpin")
	}
}

var sortPinInfoByCid = func(p []api.PinInfo) {
	sort.Slice(p, func(i, j int) bool {
		return p[i].Cid.String() < p[j].Cid.String()
	})
}

func TestStatelessTracker_SyncAll(t *testing.T) {
	type args struct {
		cs      []*cid.Cid
		tracker *Tracker
	}
	tests := []struct {
		name    string
		args    args
		want    []api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless syncall",
			args{
				[]*cid.Cid{
					test.MustDecodeCid(test.TestCid1),
					test.MustDecodeCid(test.TestCid2),
				},
				testStatelessPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
		{
			"slow stateless syncall",
			args{
				[]*cid.Cid{
					test.MustDecodeCid(test.TestCid1),
					test.MustDecodeCid(test.TestCid2),
				},
				testSlowStatelessPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.tracker.SyncAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.SyncAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != 0 {
				t.Fatalf("should not have synced anything when it tracks nothing")
			}

			for _, c := range tt.args.cs {
				err := tt.args.tracker.Track(api.Pin{Cid: c, ReplicationFactorMax: -1})
				if err != nil {
					t.Fatal(err)
				}
				tt.args.tracker.optracker.SetError(c, errors.New("test error"))
			}

			got, err = tt.args.tracker.SyncAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.SyncAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			sortPinInfoByCid(got)
			sortPinInfoByCid(tt.want)

			for i := range got {
				if got[i].Cid.String() != tt.want[i].Cid.String() {
					t.Errorf("got: %v\n want %v", got[i].Cid.String(), tt.want[i].Cid.String())
				}

				if got[i].Status != tt.want[i].Status {
					t.Errorf("got: %v\n want %v", got[i].Status, tt.want[i].Status)
				}
			}
		})
	}
}
