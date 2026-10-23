package arxml

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Settings struct {
	Package       string                    `json:"package"`
	Role          string                    `json:"role"`
	Application   string                    `json:"application"`
	ApplicationID any                       `json:"application_id"`
	Unicast       string                    `json:"unicast"`
	Transport     string                    `json:"transport"`
	Port          any                       `json:"port"`
	ServiceID     any                       `json:"service_id"`
	InstanceID    any                       `json:"instance_id"`
	InstanceName  string                    `json:"instance_name"`
	Major         any                       `json:"major"`
	Minor         any                       `json:"minor"`
	Members       map[string]MemberSettings `json:"members"`
	TypeMapping   map[string]string         `json:"type_mapping"`
}
type MemberSettings struct {
	ID          any   `json:"id,omitempty"`
	Getter      any   `json:"getter_id,omitempty"`
	Setter      any   `json:"setter_id,omitempty"`
	Notifier    any   `json:"notifier_id,omitempty"`
	EventGroups []any `json:"eventgroups,omitempty"`
}
type Bundle struct {
	Filename string            `json:"filename"`
	Data     string            `json:"data"`
	Files    map[string]string `json:"files"`
}

func Template(s *Service) Settings {
	cfg := Settings{Package: "generated", Role: "server", Application: "arxml_app", Transport: "udp", InstanceName: "generated." + s.Name, Members: map[string]MemberSettings{}, TypeMapping: map[string]string{}}
	var dep *Deployment
	if len(s.Deployments) == 1 {
		dep = &s.Deployments[0]
		cfg.ServiceID = dep.ServiceID
		cfg.Major = dep.Major
		cfg.Minor = dep.Minor
	}
	for kind, list := range map[string][]Member{"METHOD": s.Methods, "EVENT": s.Events, "FIELD": s.Fields} {
		for _, m := range list {
			mc := MemberSettings{EventGroups: []any{}}
			if dep != nil && kind != "FIELD" {
				dep.Definition.Walk(func(n *Node) {
					if n.Tag == "SOMEIP-"+kind+"-DEPLOYMENT" && n.Value(kind+"-REF") == m.Path {
						mc.ID = n.Value(kind + "-ID")
					}
				})
			}
			cfg.Members[m.Name] = mc
		}
	}
	return cfg
}

type exportBuilder struct {
	err   error
	used  map[int64]bool
	cfg   Settings
	types map[string]*Node
}

func (b *exportBuilder) fail(format string, args ...any) {
	if b.err == nil {
		b.err = fmt.Errorf(format, args...)
	}
}
func (b *exportBuilder) number(v any, label string, low, high int64) int64 {
	text := fmt.Sprint(v)
	base := 10
	if strings.HasPrefix(strings.ToLower(text), "0x") {
		base = 16
		text = text[2:]
	}
	n, e := strconv.ParseInt(text, base, 64)
	if e != nil || n < low || n > high {
		b.fail("%s 必须为 %d–%d 内整数（支持 0x 十六进制）", label, low, high)
	}
	return n
}

var ident = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var reserved = map[string]bool{"interface": true, "method": true, "broadcast": true, "attribute": true, "version": true, "in": true, "out": true, "package": true, "struct": true, "enumeration": true, "array": true, "union": true, "typedef": true, "import": true, "readonly": true, "noSubscriptions": true, "fireAndForget": true}

func (b *exportBuilder) identifier(s string) string {
	if !ident.MatchString(s) || reserved[s] {
		b.fail("不支持的 Franca 标识符：%s", s)
	}
	return s
}
func (b *exportBuilder) wire(v any, label string, event bool) int64 {
	low, high := int64(1), int64(32767)
	if event {
		low = 32768
		high = 65534
	}
	n := b.number(v, label, low, high)
	if b.used[n] {
		b.fail("重复的 method/event ID：%d", n)
	}
	b.used[n] = true
	return n
}
func first(values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	return ""
}
func (b *exportBuilder) resolve(ref string, seen map[string]bool) string {
	if mapped := b.cfg.TypeMapping[ref]; mapped != "" {
		switch mapped {
		case "UInt8", "UInt16", "UInt32", "UInt64", "Int8", "Int16", "Int32", "Int64", "Boolean", "Float", "Double", "String":
			return mapped
		}
		b.fail("类型映射仅支持 Franca 基础类型：%s", ref)
		return ""
	}
	n := b.types[ref]
	if n == nil || seen[ref] {
		b.fail("无法解析类型 %s；请在 type_mapping 中明确指定基础类型", ref)
		return ""
	}
	seen[ref] = true
	category := first(n.Texts("CATEGORY"))
	if category != "" && category != "VALUE" && category != "TYPE_REFERENCE" && category != "FIXED_LENGTH" {
		b.fail("暂不支持复杂类型：%s", ref)
		return ""
	}
	for _, key := range []string{"IMPLEMENTATION-DATA-TYPE-REF", "BASE-TYPE-REF"} {
		refs := n.Texts(key)
		if len(refs) == 1 {
			return b.resolve(refs[0], seen)
		}
	}
	encoding, size := first(n.Texts("BASE-TYPE-ENCODING")), first(n.Texts("BASE-TYPE-SIZE"))
	if size == "8" || size == "16" || size == "32" || size == "64" {
		if encoding == "NONE" {
			return "UInt" + size
		}
		if encoding == "2C" {
			return "Int" + size
		}
	}
	if encoding == "BOOLEAN" && size != "" {
		return "Boolean"
	}
	if encoding == "IEEE754" {
		if size == "32" {
			return "Float"
		}
		if size == "64" {
			return "Double"
		}
	}
	b.fail("无法可靠映射类型 %s；请填写 type_mapping", ref)
	return ""
}
func pretty(v any) string { data, _ := json.MarshalIndent(v, "", "  "); return string(data) }
func Export(doc *Document, path string, cfg Settings) (Bundle, error) {
	s := doc.Service(path)
	if s == nil {
		return Bundle{}, fmt.Errorf("请选择要导出的服务")
	}
	b := &exportBuilder{used: map[int64]bool{}, cfg: cfg, types: map[string]*Node{}}
	if s.Kind == "SENDER-RECEIVER-INTERFACE" {
		b.fail("Sender/Receiver 暂不支持转换为 CommonAPI 服务")
	}
	if cfg.Role != "server" && cfg.Role != "client" {
		b.fail("角色必须是 server 或 client")
	}
	name := b.identifier(s.Name)
	for _, p := range strings.Split(cfg.Package, ".") {
		b.identifier(p)
	}
	app := b.identifier(cfg.Application)
	sid := b.number(cfg.ServiceID, "Service ID", 1, 65534)
	iid := b.number(cfg.InstanceID, "Instance ID", 1, 65534)
	aid := b.number(cfg.ApplicationID, "Application ID", 1, 65534)
	major := b.number(cfg.Major, "Major version", 0, 254)
	minor := b.number(cfg.Minor, "Minor version", 0, 4294967294)
	ip := net.ParseIP(cfg.Unicast)
	if ip == nil || ip.To4() == nil || ip.IsMulticast() || ip.IsUnspecified() {
		b.fail("unicast 必须为本机 IPv4 单播地址")
	}
	if cfg.Transport != "tcp" && cfg.Transport != "udp" {
		b.fail("transport 必须是 tcp 或 udp")
	}
	reliable := strconv.FormatBool(cfg.Transport == "tcp")
	var port int64
	if cfg.Role == "server" {
		port = b.number(cfg.Port, "服务端口", 1, 65535)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(cfg.InstanceName) {
		b.fail("请填写合法的 CommonAPI instance_name")
	}
	if b.err != nil {
		return Bundle{}, b.err
	}
	for _, t := range doc.Types {
		b.types[t.Path] = t.Definition
	}
	var fidl, fdepl strings.Builder
	fmt.Fprintf(&fidl, "package %s\n\ninterface %s {\n    version { major %d minor %d }\n", cfg.Package, name, major, minor)
	fmt.Fprintf(&fdepl, "import \"platform:/plugin/org.genivi.commonapi.someip/deployment/CommonAPI-SOMEIP_deployment_spec.fdepl\"\nimport \"%s.fidl\"\n\ndefine org.genivi.commonapi.someip.deployment for interface %s.%s {\n    SomeIpServiceID = %d\n", name, cfg.Package, name, sid)
	events := []map[string]string{}
	groups := map[int64][]string{}
	eventGroup := func(mc MemberSettings, eid int64, field bool) string {
		if len(mc.EventGroups) == 0 {
			b.fail("事件或 notifier 必须填写 eventgroups")
		}
		ids := []int64{}
		seen := map[int64]bool{}
		for _, v := range mc.EventGroups {
			n := b.number(v, "Eventgroup ID", 1, 65534)
			if !seen[n] {
				ids = append(ids, n)
				seen[n] = true
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		parts := []string{}
		for _, n := range ids {
			parts = append(parts, strconv.FormatInt(n, 10))
			groups[n] = append(groups[n], fmt.Sprintf("0x%04x", eid))
		}
		events = append(events, map[string]string{"event": fmt.Sprintf("0x%04x", eid), "is_field": strconv.FormatBool(field), "is_reliable": reliable})
		return strings.Join(parts, ", ")
	}
	memberNames := map[string]bool{}
	for _, list := range [][]Member{s.Methods, s.Events, s.Fields} {
		for _, m := range list {
			b.identifier(m.Name)
			if memberNames[m.Name] {
				b.fail("重复的成员名：%s", m.Name)
			}
			memberNames[m.Name] = true
			if len(m.Definition.Texts("POSSIBLE-ERROR-REF"))+len(m.Definition.Texts("APPLICATION-ERROR-REF")) > 0 {
				b.fail("%s 包含尚不支持的应用错误定义", m.Name)
			}
		}
	}
	for _, m := range s.Methods {
		mc := cfg.Members[m.Name]
		mid := b.wire(mc.ID, m.Name+" Method ID", false)
		fire := first(m.Definition.Texts("FIRE-AND-FORGET")) == "true"
		mod := ""
		if fire {
			mod = " fireAndForget"
		}
		fmt.Fprintf(&fidl, "    method %s%s {\n", m.Name, mod)
		for _, a := range m.Arguments {
			if a.Direction != "IN" && a.Direction != "OUT" {
				b.fail("%s 参数方向 %s 暂不支持", m.Name, a.Direction)
			}
		}
		for _, direction := range []string{"IN", "OUT"} {
			args := []Member{}
			for _, a := range m.Arguments {
				if a.Direction == direction {
					args = append(args, a)
				}
			}
			if len(args) > 0 {
				if fire && direction == "OUT" {
					b.fail("fireAndForget 方法不能有 OUT 参数")
				}
				fmt.Fprintf(&fidl, "        %s {\n", strings.ToLower(direction))
				for _, a := range args {
					fmt.Fprintf(&fidl, "            %s %s\n", b.resolve(a.TypeRef, map[string]bool{}), b.identifier(a.Name))
				}
				fidl.WriteString("        }\n")
			}
		}
		fidl.WriteString("    }\n")
		fmt.Fprintf(&fdepl, "    method %s { SomeIpMethodID = %d SomeIpReliable = %s }\n", m.Name, mid, reliable)
	}
	for _, m := range s.Events {
		mc := cfg.Members[m.Name]
		eid := b.wire(mc.ID, m.Name+" Event ID", true)
		gs := eventGroup(mc, eid, false)
		fmt.Fprintf(&fidl, "    broadcast %s { out { %s value } }\n", m.Name, b.resolve(m.TypeRef, map[string]bool{}))
		fmt.Fprintf(&fdepl, "    broadcast %s { SomeIpEventID = %d SomeIpReliable = %s SomeIpEventGroups = { %s } }\n", m.Name, eid, reliable, gs)
	}
	for _, m := range s.Fields {
		mc := cfg.Members[m.Name]
		if m.Access["HAS-GETTER"] != "true" {
			b.fail("%s 无 getter，暂不支持转换为 CommonAPI attribute", m.Name)
		}
		setter, notifier := m.Access["HAS-SETTER"] == "true", m.Access["HAS-NOTIFIER"] == "true"
		mods := ""
		if !setter {
			mods += " readonly"
		}
		if !notifier {
			mods += " noSubscriptions"
		}
		fmt.Fprintf(&fidl, "    attribute %s %s%s\n", b.resolve(m.TypeRef, map[string]bool{}), m.Name, mods)
		fmt.Fprintf(&fdepl, "    attribute %s {\n", m.Name)
		for _, access := range []struct {
			label   string
			enabled bool
			id      any
		}{{"Getter", true, mc.Getter}, {"Setter", setter, mc.Setter}, {"Notifier", notifier, mc.Notifier}} {
			if access.enabled {
				wid := b.wire(access.id, m.Name+" "+access.label, access.label == "Notifier")
				fmt.Fprintf(&fdepl, "        SomeIp%sID = %d SomeIp%sReliable = %s\n", access.label, wid, access.label, reliable)
				if access.label == "Notifier" {
					fmt.Fprintf(&fdepl, "        SomeIpEventGroups = { %s }\n", eventGroup(mc, wid, true))
				}
			}
		}
		fdepl.WriteString("    }\n")
	}
	if b.err != nil {
		return Bundle{}, b.err
	}
	fidl.WriteString("}\n")
	fmt.Fprintf(&fdepl, "}\n\ndefine org.genivi.commonapi.someip.deployment for provider as Service {\n    instance %s.%s {\n        InstanceId = \"%s\"\n        SomeIpInstanceID = %d\n    }\n}\n", cfg.Package, name, cfg.InstanceName, iid)
	config := map[string]any{"unicast": cfg.Unicast, "applications": []any{map[string]string{"name": app, "id": fmt.Sprintf("0x%04x", aid)}}, "routing": app, "service-discovery": map[string]string{"enable": "true", "multicast": "224.224.224.245", "port": "30490", "protocol": "udp"}}
	if cfg.Role == "server" {
		keys := []int64{}
		for g := range groups {
			keys = append(keys, g)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		egs := []any{}
		for _, g := range keys {
			egs = append(egs, map[string]any{"eventgroup": fmt.Sprintf("0x%04x", g), "events": groups[g]})
		}
		entry := map[string]any{"service": fmt.Sprintf("0x%04x", sid), "instance": fmt.Sprintf("0x%04x", iid), "events": events, "eventgroups": egs}
		if cfg.Transport == "tcp" {
			entry["reliable"] = map[string]string{"port": strconv.FormatInt(port, 10)}
		} else {
			entry["unreliable"] = strconv.FormatInt(port, 10)
		}
		config["services"] = []any{entry}
	}
	instruction := "Implement the generated StubDefault and register the service."
	if cfg.Role == "client" {
		instruction = "Use the generated Proxy and buildProxy with the configured InstanceId."
	}
	files := map[string]string{name + ".fidl": fidl.String(), name + ".fdepl": fdepl.String(), "vsomeip-" + cfg.Role + ".json": pretty(config), "export-settings.json": pretty(cfg), "README.txt": fmt.Sprintf("Source: %s\nService: %s\nRole: %s\nRun commonapi-core-generator %s.fidl and commonapi-someip-generator %s.fdepl.\n%s\nSet VSOMEIP_APPLICATION_NAME=%s and VSOMEIP_CONFIGURATION to vsomeip-%s.json.\nService discovery multicast: 224.224.224.245:30490; adapt to your network.\nMultiple apps on one host require a shared routing manager configuration.\nScalar types only. Verify serialization/deployment against your target ECU.\nGenerated files require your CommonAPI/vsomeip toolchain and application implementation.\n", doc.Source, path, cfg.Role, name, name, instruction, app, cfg.Role)}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := []string{}
	for f := range files {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		w, e := zw.Create(f)
		if e != nil {
			return Bundle{}, e
		}
		if _, e = w.Write([]byte(files[f])); e != nil {
			return Bundle{}, e
		}
	}
	if e := zw.Close(); e != nil {
		return Bundle{}, e
	}
	return Bundle{Filename: name + "-" + cfg.Role + ".zip", Data: base64.StdEncoding.EncodeToString(buf.Bytes()), Files: files}, nil
}
