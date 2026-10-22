package devices

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.yaml")

	// Missing file -> empty store.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open empty: %v", err)
	}
	if !s.Empty() {
		t.Fatal("expected empty store")
	}

	if err := s.UpsertPrinter(PrinterRecord{ID: "qidi", Kind: "moonraker", Name: "Qidi", BaseURL: "http://192.168.0.76"}); err != nil {
		t.Fatalf("upsert printer: %v", err)
	}
	if err := s.UpsertCamera(CameraRecord{Name: "garage", Src: "rtsp://x/y", Record: true}); err != nil {
		t.Fatalf("upsert camera: %v", err)
	}

	// Reload from disk and confirm persistence.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.Printers(); len(got) != 1 || got[0].ID != "qidi" || got[0].BaseURL != "http://192.168.0.76" {
		t.Fatalf("printers not persisted: %+v", got)
	}
	if got := s2.Cameras(); len(got) != 1 || got[0].Name != "garage" || !got[0].Record {
		t.Fatalf("cameras not persisted: %+v", got)
	}

	// Upsert replaces by key.
	if err := s2.UpsertPrinter(PrinterRecord{ID: "qidi", Kind: "moonraker", Name: "Qidi Plus 4", BaseURL: "http://192.168.0.76"}); err != nil {
		t.Fatalf("replace printer: %v", err)
	}
	if got := s2.Printers(); len(got) != 1 || got[0].Name != "Qidi Plus 4" {
		t.Fatalf("upsert did not replace: %+v", got)
	}

	// Delete.
	if ok, err := s2.DeletePrinter("qidi"); err != nil || !ok {
		t.Fatalf("delete printer: ok=%v err=%v", ok, err)
	}
	if ok, _ := s2.DeletePrinter("qidi"); ok {
		t.Fatal("second delete should report not-found")
	}
	if got := s2.Printers(); len(got) != 0 {
		t.Fatalf("printer not deleted: %+v", got)
	}
}

func TestValidatePrinter(t *testing.T) {
	good := []PrinterRecord{
		{ID: "p1", Kind: "moonraker", Name: "P1", BaseURL: "http://10.0.0.2:7125"},
		{ID: "mk4", Kind: "prusalink", Name: "MK4", BaseURL: "http://10.0.0.3", APIKey: "pw"},
	}
	for i, r := range good {
		if err := ValidatePrinter(r); err != nil {
			t.Fatalf("valid printer #%d rejected: %v", i, err)
		}
	}
	bad := []PrinterRecord{
		{ID: "Bad Id", Kind: "moonraker", Name: "x", BaseURL: "http://x"},
		{ID: "p1", Kind: "moonraker", Name: "", BaseURL: "http://x"},
		{ID: "p1", Kind: "moonraker", Name: "x", BaseURL: "ftp://x"},
		{ID: "p1", Kind: "octoprint", Name: "x", BaseURL: "http://x"}, // missing api key
		{ID: "p1", Kind: "prusalink", Name: "x", BaseURL: "http://x"}, // missing password
		{ID: "p1", Kind: "prusalink", Name: "x", APIKey: "pw"},        // missing base_url
		{ID: "p1", Kind: "bambu", Name: "x"},                          // missing ip/serial/code
		{ID: "p1", Kind: "weird", Name: "x", BaseURL: "http://x"},
	}
	for i, r := range bad {
		if err := ValidatePrinter(r); err == nil {
			t.Fatalf("bad printer #%d accepted: %+v", i, r)
		}
	}
}

func TestValidateCameraName(t *testing.T) {
	for _, n := range []string{"garage", "front_door", "cam1"} {
		if err := ValidateCameraName(n); err != nil {
			t.Fatalf("valid name %q rejected: %v", n, err)
		}
	}
	for _, n := range []string{"Garage", "front-door", "cam 1", "", "printer1_hq"} {
		if err := ValidateCameraName(n); err == nil {
			t.Fatalf("bad name %q accepted", n)
		}
	}
}
