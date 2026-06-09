package main

import (
	_ "embed"
	"encoding/xml"
	"strings"
	"testing"
)

//go:embed sieganet-client.manifest
var manifestXML string

// TestManifestRequiresAdministrator parses the embedded application manifest and
// confirms it requests elevation, so a double-click prompts for UAC (TUN, routes,
// WFP and NRPT all need admin). The manifest is compiled into the Windows exe via
// the committed rsrc_windows_*.syso resources.
func TestManifestRequiresAdministrator(t *testing.T) {
	type rel struct {
		Level string `xml:"level,attr"`
	}
	var doc struct {
		RELs []rel `xml:"trustInfo>security>requestedPrivileges>requestedExecutionLevel"`
	}
	if err := xml.Unmarshal([]byte(manifestXML), &doc); err != nil {
		t.Fatalf("manifest is not valid XML: %v", err)
	}
	if len(doc.RELs) != 1 {
		t.Fatalf("expected one requestedExecutionLevel, got %d", len(doc.RELs))
	}
	if doc.RELs[0].Level != "requireAdministrator" {
		t.Fatalf("execution level = %q, want requireAdministrator", doc.RELs[0].Level)
	}
	if !strings.Contains(manifestXML, "uiAccess=\"false\"") {
		t.Error("manifest should set uiAccess=false")
	}
}
