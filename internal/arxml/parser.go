package arxml

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf16"
)

const MaxBytes = 20 * 1024 * 1024

var unordered = map[string]bool{"METHODS": true, "OPERATIONS": true, "EVENTS": true, "FIELDS": true, "DATA-ELEMENTS": true, "METHOD-DEPLOYMENTS": true, "EVENT-DEPLOYMENTS": true, "FIELD-DEPLOYMENTS": true, "EVENT-GROUPS": true}

// Convert BOM-marked UTF-16 before decoding; UTF-8 is otherwise required.
func xmlBytes(data []byte) ([]byte, error) {
	if len(data) >= 2 && (bytes.HasPrefix(data, []byte{255, 254}) || bytes.HasPrefix(data, []byte{254, 255})) {
		if len(data)%2 != 0 {
			return nil, fmt.Errorf("UTF-16 字节数无效")
		}
		var order binary.ByteOrder = binary.BigEndian
		if data[0] == 255 {
			order = binary.LittleEndian
		}
		units := make([]uint16, (len(data)-2)/2)
		for i := range units {
			units[i] = order.Uint16(data[2+i*2:])
		}
		data = []byte(string(utf16.Decode(units)))
	}
	return bytes.TrimPrefix(data, []byte{239, 187, 191}), nil
}
func Parse(data []byte, filename string) (*Document, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("文件超过 20 MiB 限制")
	}
	data, err := xmlBytes(data)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(charset string, r io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "utf-16") || strings.EqualFold(charset, "utf-8") {
			return r, nil
		}
		return nil, fmt.Errorf("不支持字符编码 %s", charset)
	}
	var root *Node
	stack := []*Node{}
	for {
		token, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, fmt.Errorf("XML 格式错误：%w", e)
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, fmt.Errorf("不支持 DTD 或实体声明")
		case xml.StartElement:
			if len(stack) >= 150 {
				return nil, fmt.Errorf("XML 嵌套超过 150 层")
			}
			n := &Node{Tag: t.Name.Local, Attributes: map[string]string{}, Children: []*Node{}}
			for _, a := range t.Attr {
				if a.Name.Local != "UUID" && a.Name.Local != "xmlns" && a.Name.Space != "xmlns" {
					key := a.Name.Local
					if a.Name.Space != "" {
						key = "{" + a.Name.Space + "}" + key
					}
					n.Attributes[key] = a.Value
				}
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("XML 只能有一个根元素")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			n := stack[len(stack)-1]
			n.Text = strings.TrimSpace(n.Text)
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, fmt.Errorf("XML 根元素外存在文本")
			}
		}
	}
	if root == nil || root.Tag != "AUTOSAR" {
		return nil, fmt.Errorf("根元素必须为 AUTOSAR")
	}
	index := map[string]*Node{}
	var walk func(*Node, string) error
	walk = func(n *Node, parent string) error {
		n.Path = parent
		if name := n.Value("SHORT-NAME"); name != "" {
			n.Path += "/" + name
			if index[n.Path] != nil {
				return fmt.Errorf("重复的 ARXML 路径：%s", n.Path)
			}
			index[n.Path] = n
		}
		for _, c := range n.Children {
			if e := walk(c, n.Path); e != nil {
				return e
			}
		}
		if unordered[n.Tag] {
			sort.SliceStable(n.Children, func(i, j int) bool {
				return n.Children[i].Tag+n.Children[i].Value("SHORT-NAME") < n.Children[j].Tag+n.Children[j].Value("SHORT-NAME")
			})
		}
		return nil
	}
	if err = walk(root, ""); err != nil {
		return nil, err
	}
	d := &Document{Format: "arxml-ops/v1", Source: filename, Services: []Service{}, Types: []DataType{}, Warnings: []Warning{}, Summary: map[string]int{}}
	member := func(n *Node) Member {
		ref := n.Value("TYPE-TREF")
		if ref == "" {
			ref = n.Value("TYPE-REF")
		}
		return Member{Name: n.Value("SHORT-NAME"), Path: n.Path, TypeRef: ref, Direction: n.Value("DIRECTION"), Definition: n}
	}
	root.Walk(func(n *Node) {
		if n.Tag != "SERVICE-INTERFACE" && n.Tag != "CLIENT-SERVER-INTERFACE" && n.Tag != "SENDER-RECEIVER-INTERFACE" {
			return
		}
		s := Service{Name: n.Value("SHORT-NAME"), Path: n.Path, Kind: n.Tag, IsService: n.Value("IS-SERVICE"), Definition: n, Methods: []Member{}, Events: []Member{}, Fields: []Member{}, DataElements: []Member{}, Deployments: []Deployment{}}
		for _, c := range n.Children {
			var target *[]Member
			switch c.Tag {
			case "METHODS", "OPERATIONS":
				target = &s.Methods
			case "EVENTS":
				target = &s.Events
			case "FIELDS":
				target = &s.Fields
			case "DATA-ELEMENTS":
				target = &s.DataElements
			default:
				continue
			}
			for _, v := range c.Children {
				m := member(v)
				if a := v.Child("ARGUMENTS"); a != nil {
					for _, arg := range a.Children {
						m.Arguments = append(m.Arguments, member(arg))
					}
				}
				if c.Tag == "FIELDS" {
					m.Access = map[string]string{}
					for _, k := range []string{"HAS-GETTER", "HAS-SETTER", "HAS-NOTIFIER"} {
						m.Access[k] = v.Value(k)
					}
				}
				*target = append(*target, m)
			}
		}
		d.Services = append(d.Services, s)
	})
	root.Walk(func(n *Node) {
		if n.Tag == "SOMEIP-SERVICE-INTERFACE-DEPLOYMENT" {
			if s := d.Service(n.Value("SERVICE-INTERFACE-REF")); s != nil {
				version := n.Child("SERVICE-INTERFACE-VERSION")
				s.Deployments = append(s.Deployments, Deployment{Name: n.Value("SHORT-NAME"), Path: n.Path, ServiceID: n.Value("SERVICE-INTERFACE-ID"), Major: version.Value("MAJOR-VERSION"), Minor: version.Value("MINOR-VERSION"), Definition: n})
			}
		}
		if (strings.HasSuffix(n.Tag, "-REF") || strings.HasSuffix(n.Tag, "-TREF")) && strings.HasPrefix(n.Text, "/") && index[n.Text] == nil {
			d.Warnings = append(d.Warnings, Warning{Path: n.Path, Reference: n.Text, Message: "引用未在当前文件中找到，可能位于其他 ARXML 文件"})
		}
	})
	for path, n := range index {
		if strings.HasSuffix(n.Tag, "-DATA-TYPE") || n.Tag == "SW-BASE-TYPE" || n.Tag == "COMPU-METHOD" || n.Tag == "DATA-CONSTR" {
			d.Types = append(d.Types, DataType{Path: path, Name: n.Value("SHORT-NAME"), Kind: n.Tag, Definition: n})
		}
	}
	sort.Slice(d.Services, func(i, j int) bool { return d.Services[i].Path < d.Services[j].Path })
	sort.Slice(d.Types, func(i, j int) bool { return d.Types[i].Path < d.Types[j].Path })
	for i := range d.Services {
		s := &d.Services[i]
		sort.Slice(s.Deployments, func(i, j int) bool { return s.Deployments[i].Path < s.Deployments[j].Path })
		d.Summary["methods"] += len(s.Methods)
		d.Summary["events"] += len(s.Events)
		d.Summary["fields"] += len(s.Fields)
		d.Summary["data_elements"] += len(s.DataElements)
	}
	d.Summary["services"] = len(d.Services)
	d.Summary["types"] = len(d.Types)
	if len(d.Services) == 0 {
		d.Warnings = append(d.Warnings, Warning{Message: "未找到支持的服务或通信接口；文件可能只包含部署或数据类型"})
	}
	return d, nil
}
