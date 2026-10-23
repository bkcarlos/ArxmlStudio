package arxml

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("../../examples/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func parse(t *testing.T, data []byte) *Document {
	t.Helper()
	d, e := Parse(data, "input.arxml")
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func settings(t *testing.T) Settings {
	t.Helper()
	var cfg Settings
	if e := json.Unmarshal(fixture(t, "export-settings.json"), &cfg); e != nil {
		t.Fatal(e)
	}
	return cfg
}
func TestParse(t *testing.T) {
	d := parse(t, fixture(t, "vehicle-v1.arxml"))
	if d.Summary["services"] != 1 || d.Summary["methods"] != 1 || d.Summary["events"] != 1 || d.Summary["fields"] != 1 || len(d.Warnings) != 0 {
		t.Fatalf("unexpected document: %+v", d.Summary)
	}
	if d.Services[0].Methods[0].Arguments[0].Direction != "IN" {
		t.Fatal("missing direction")
	}
}
func TestFormatNoise(t *testing.T) {
	data := fixture(t, "vehicle-v1.arxml")
	other := bytes.ReplaceAll(data, []byte(` xmlns="http://autosar.org/schema/r4.0"`), nil)
	other = bytes.ReplaceAll(other, []byte("<SERVICE-INTERFACE>"), []byte(`<SERVICE-INTERFACE UUID="changed">`))
	other = bytes.ReplaceAll(other, []byte("><"), []byte(">\n <"))
	if diff := Compare(parse(t, data), parse(t, other)); len(diff.Changes) != 0 {
		t.Fatalf("formatting changes: %+v", diff)
	}
}
func TestUTF16(t *testing.T) {
	text := strings.Replace(string(fixture(t, "vehicle-v1.arxml")), "UTF-8", "UTF-16", 1)
	var data bytes.Buffer
	data.Write([]byte{255, 254})
	for _, v := range utf16.Encode([]rune(text)) {
		_ = binary.Write(&data, binary.LittleEndian, v)
	}
	d := parse(t, data.Bytes())
	if len(d.Services) != 1 {
		t.Fatal("UTF-16 decode failed")
	}
}
func TestDiff(t *testing.T) {
	a := parse(t, fixture(t, "vehicle-v1.arxml"))
	b := parse(t, fixture(t, "vehicle-v2.arxml"))
	diff := Compare(a, b)
	if diff.Summary["added"] != 1 || diff.Summary["modified"] != 2 {
		t.Fatalf("unexpected diff: %+v", diff)
	}
	found := false
	for _, c := range diff.Changes {
		if strings.Contains(c.Path, "METHOD-ID") && c.Before == "1" && c.After == "5" {
			found = true
		}
	}
	if !found {
		t.Fatal("method ID change not isolated")
	}
}
func TestArgumentOrder(t *testing.T) {
	data := fixture(t, "vehicle-v1.arxml")
	arg := []byte("<ARGUMENT-DATA-PROTOTYPE><SHORT-NAME>second</SHORT-NAME><TYPE-TREF>/Vehicle/Speed</TYPE-TREF><DIRECTION>IN</DIRECTION></ARGUMENT-DATA-PROTOTYPE>")
	a := bytes.Replace(data, []byte("<ARGUMENTS>"), append([]byte("<ARGUMENTS>"), arg...), 1)
	b := bytes.Replace(data, []byte("</ARGUMENTS>"), append(arg, []byte("</ARGUMENTS>")...), 1)
	if len(Compare(parse(t, a), parse(t, b)).Changes) == 0 {
		t.Fatal("argument order lost")
	}
}
func TestInvalidXML(t *testing.T) {
	for _, data := range []string{"<broken>", "<root/>", "<!DOCTYPE AUTOSAR><AUTOSAR/>", "<AUTOSAR/><AUTOSAR/>", "text<AUTOSAR/>", "<AUTOSAR>&unknown;</AUTOSAR>", "<AUTOSAR>" + strings.Repeat("<N>", 151) + strings.Repeat("</N>", 151) + "</AUTOSAR>"} {
		if _, e := Parse([]byte(data), "bad"); e == nil {
			t.Errorf("accepted invalid XML %.80s", data)
		}
	}
}
func TestDuplicatePaths(t *testing.T) {
	data := fixture(t, "vehicle-v1.arxml")
	data = bytes.Replace(data, []byte("</ELEMENTS>"), []byte("<SERVICE-INTERFACE><SHORT-NAME>VehicleService</SHORT-NAME></SERVICE-INTERFACE></ELEMENTS>"), 1)
	if _, e := Parse(data, "duplicate"); e == nil {
		t.Fatal("accepted duplicate path")
	}
}
func TestReferencesAndClassic(t *testing.T) {
	data := fixture(t, "vehicle-v1.arxml")
	d := parse(t, bytes.ReplaceAll(data, []byte("/Vehicle/Speed"), []byte("/Missing/Type")))
	if len(d.Warnings) == 0 {
		t.Fatal("missing warning")
	}
	classic := bytes.ReplaceAll(data, []byte("SERVICE-INTERFACE>"), []byte("CLIENT-SERVER-INTERFACE>"))
	classic = bytes.ReplaceAll(classic, []byte("METHODS>"), []byte("OPERATIONS>"))
	d = parse(t, classic)
	if d.Services[0].Kind != "CLIENT-SERVER-INTERFACE" || len(d.Services[0].Methods) != 1 {
		t.Fatal("Classic interface not parsed")
	}
}
func TestTemplate(t *testing.T) {
	d := parse(t, fixture(t, "vehicle-v1.arxml"))
	cfg := Template(&d.Services[0])
	if cfg.ServiceID != "0x1234" || cfg.Members["SetSpeed"].ID != "1" || cfg.InstanceID != nil {
		t.Fatalf("unexpected template %+v", cfg)
	}
}
func TestExportRolesAndTransport(t *testing.T) {
	d := parse(t, fixture(t, "vehicle-v1.arxml"))
	for _, role := range []string{"server", "client"} {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(role+"/"+transport, func(t *testing.T) {
				cfg := settings(t)
				cfg.Role = role
				cfg.Transport = transport
				if role == "client" {
					cfg.Port = nil
				}
				bundle, e := Export(d, "/Vehicle/VehicleService", cfg)
				if e != nil {
					t.Fatal(e)
				}
				raw, e := base64.StdEncoding.DecodeString(bundle.Data)
				if e != nil {
					t.Fatal(e)
				}
				zr, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
				if e != nil {
					t.Fatal(e)
				}
				if len(zr.File) != 5 {
					t.Fatal("missing archive files")
				}
				for _, f := range zr.File {
					r, e := f.Open()
					if e != nil {
						t.Fatal(e)
					}
					data, e := io.ReadAll(r)
					r.Close()
					if e != nil {
						t.Fatal(e)
					}
					if string(data) != bundle.Files[f.Name] {
						t.Fatal("archive content mismatch")
					}
				}
				var config map[string]any
				if e = json.Unmarshal([]byte(bundle.Files["vsomeip-"+role+".json"]), &config); e != nil {
					t.Fatal(e)
				}
				if role == "client" && config["services"] != nil {
					t.Fatal("client must not provide services")
				}
				if role == "server" {
					s := config["services"].([]any)[0].(map[string]any)
					if s["service"] != "0x1234" {
						t.Fatal("service ID mismatch")
					}
					if transport == "tcp" && s["reliable"] == nil {
						t.Fatal("TCP endpoint missing")
					}
					if transport == "udp" && s["unreliable"] != "30509" {
						t.Fatal("UDP port mismatch")
					}
				}
				if !strings.Contains(bundle.Files["VehicleService.fidl"], "UInt16 speed") || !strings.Contains(bundle.Files["VehicleService.fdepl"], "SomeIpGetterID = 2") {
					t.Fatal("missing interface definitions")
				}
			})
		}
	}
}
func TestExportValidation(t *testing.T) {
	d := parse(t, fixture(t, "vehicle-v1.arxml"))
	cases := map[string]func(*Settings){"missing ID": func(c *Settings) { m := c.Members["SetSpeed"]; m.ID = nil; c.Members["SetSpeed"] = m }, "duplicate ID": func(c *Settings) { m := c.Members["SetSpeed"]; m.ID = 2; c.Members["SetSpeed"] = m }, "event range": func(c *Settings) { m := c.Members["SpeedChanged"]; m.ID = 1; c.Members["SpeedChanged"] = m }, "group missing": func(c *Settings) { m := c.Members["SpeedChanged"]; m.EventGroups = nil; c.Members["SpeedChanged"] = m }, "bad IPv4": func(c *Settings) { c.Unicast = "224.0.0.1" }, "bad role": func(c *Settings) { c.Role = "invalid" }, "bad name": func(c *Settings) { c.Package = "bad/name" }, "bad minor": func(c *Settings) { c.Minor = -1 }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := settings(t)
			change(&cfg)
			if _, e := Export(d, "/Vehicle/VehicleService", cfg); e == nil {
				t.Fatal("invalid settings accepted")
			}
		})
	}
	unknown := parse(t, bytes.ReplaceAll(fixture(t, "vehicle-v1.arxml"), []byte("/Vehicle/Speed"), []byte("/Missing/Type")))
	if _, e := Export(unknown, "/Vehicle/VehicleService", settings(t)); e == nil {
		t.Fatal("unknown type silently exported")
	}
	cfg := settings(t)
	cfg.TypeMapping = map[string]string{"/Missing/Type": "UInt16"}
	if _, e := Export(unknown, "/Vehicle/VehicleService", cfg); e != nil {
		t.Fatal(e)
	}
}
func TestMethodReorderAndTypeChange(t *testing.T) {
	data := fixture(t, "vehicle-v1.arxml")
	method := []byte("<CLIENT-SERVER-OPERATION><SHORT-NAME>Reset</SHORT-NAME></CLIENT-SERVER-OPERATION>")
	a := bytes.Replace(data, []byte("<METHODS>"), append([]byte("<METHODS>"), method...), 1)
	b := bytes.Replace(data, []byte("</METHODS>"), append(method, []byte("</METHODS>")...), 1)
	if len(Compare(parse(t, a), parse(t, b)).Changes) != 0 {
		t.Fatal("method reorder is not semantic")
	}
	b = bytes.ReplaceAll(data, []byte("<BASE-TYPE-SIZE>16"), []byte("<BASE-TYPE-SIZE>32"))
	if len(Compare(parse(t, data), parse(t, b)).Changes) == 0 {
		t.Fatal("type change lost")
	}
}
