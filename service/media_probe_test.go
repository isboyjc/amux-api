package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/abema/go-mp4"
)

type contextAwareRoundTripper func(*http.Request) (*http.Response, error)

func (f contextAwareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestProbeISOBaseMediaDuration(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "duration-*.mp4")
	if err != nil {
		t.Fatalf("create test mp4: %v", err)
	}
	defer file.Close()

	writer := mp4.NewWriter(file)
	if _, err := writer.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeFtyp()}); err != nil {
		t.Fatalf("start ftyp: %v", err)
	}
	if _, err := mp4.Marshal(writer, &mp4.Ftyp{
		MajorBrand: [4]byte{'i', 's', 'o', 'm'},
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
		},
	}, mp4.Context{}); err != nil {
		t.Fatalf("marshal ftyp: %v", err)
	}
	if _, err := writer.EndBox(); err != nil {
		t.Fatalf("end ftyp: %v", err)
	}

	if _, err := writer.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMoov()}); err != nil {
		t.Fatalf("start moov: %v", err)
	}
	if _, err := writer.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMvhd()}); err != nil {
		t.Fatalf("start mvhd: %v", err)
	}
	if _, err := mp4.Marshal(writer, &mp4.Mvhd{
		Timescale:  1000,
		DurationV0: 6620,
		Rate:       0x00010000,
		Volume:     0x0100,
		Matrix: [9]int32{
			0x00010000, 0, 0,
			0, 0x00010000, 0,
			0, 0, 0x40000000,
		},
		NextTrackID: 1,
	}, mp4.Context{}); err != nil {
		t.Fatalf("marshal mvhd: %v", err)
	}
	if _, err := writer.EndBox(); err != nil {
		t.Fatalf("end mvhd: %v", err)
	}
	if _, err := writer.EndBox(); err != nil {
		t.Fatalf("end moov: %v", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek test mp4: %v", err)
	}

	duration, err := probeISOBaseMediaDuration(context.Background(), file)
	if err != nil {
		t.Fatalf("probe duration: %v", err)
	}
	if duration != 6.62 {
		t.Fatalf("duration=%v, want 6.62", duration)
	}
}

func TestDoDownloadRequestWithContextHonorsCancellation(t *testing.T) {
	originalClient := httpClient
	originalWorkerURL := system_setting.WorkerUrl
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	t.Cleanup(func() {
		httpClient = originalClient
		system_setting.WorkerUrl = originalWorkerURL
		*fetchSetting = originalFetchSetting
	})

	system_setting.WorkerUrl = ""
	fetchSetting.EnableSSRFProtection = false
	httpClient = &http.Client{Transport: contextAwareRoundTripper(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DoDownloadRequestWithContext(ctx, "https://example.com/video.mp4", "test cancellation")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("download error=%v, want context.Canceled", err)
	}
}

func TestDoDownloadRequestWithContextRejectsUninitializedClient(t *testing.T) {
	originalClient := httpClient
	originalWorkerURL := system_setting.WorkerUrl
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	t.Cleanup(func() {
		httpClient = originalClient
		system_setting.WorkerUrl = originalWorkerURL
		*fetchSetting = originalFetchSetting
	})

	system_setting.WorkerUrl = ""
	fetchSetting.EnableSSRFProtection = false
	httpClient = nil
	_, err := DoDownloadRequestWithContext(context.Background(), "https://example.com/video.mp4", "test nil client")
	if err == nil || err.Error() != "HTTP client is not initialized" {
		t.Fatalf("download error=%v, want uninitialized client error", err)
	}
}

func TestProbeRemoteVideoDurationHonorsConcurrencyWaitCancellation(t *testing.T) {
	originalSlots := remoteVideoDurationProbeSlots
	remoteVideoDurationProbeSlots = make(chan struct{}, 1)
	remoteVideoDurationProbeSlots <- struct{}{}
	t.Cleanup(func() { remoteVideoDurationProbeSlots = originalSlots })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ProbeRemoteVideoDuration(ctx, "https://example.com/video.mp4", 1024)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("probe error=%v, want context.Canceled while waiting for a slot", err)
	}
}
