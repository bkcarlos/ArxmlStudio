package arxml

// Node retains source properties not yet interpreted by the service model.
type Node struct {
	Tag        string            `json:"tag"`
	Text       string            `json:"text"`
	Attributes map[string]string `json:"attributes"`
	Children   []*Node           `json:"children"`
	Path       string            `json:"-"`
}

func (n *Node) Value(tag string) string {
	if n != nil {
		for _, c := range n.Children {
			if c.Tag == tag {
				return c.Text
			}
		}
	}
	return ""
}
func (n *Node) Child(tag string) *Node {
	if n != nil {
		for _, c := range n.Children {
			if c.Tag == tag {
				return c
			}
		}
	}
	return nil
}
func (n *Node) Walk(fn func(*Node)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		c.Walk(fn)
	}
}
func (n *Node) Texts(tag string) []string {
	out := []string{}
	n.Walk(func(c *Node) {
		if c.Tag == tag {
			out = append(out, c.Text)
		}
	})
	return out
}

type Member struct {
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	TypeRef    string            `json:"type_ref"`
	Direction  string            `json:"direction"`
	Definition *Node             `json:"definition"`
	Arguments  []Member          `json:"arguments,omitempty"`
	Access     map[string]string `json:"access,omitempty"`
}
type Deployment struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	ServiceID  string `json:"service_id"`
	Major      string `json:"major_version"`
	Minor      string `json:"minor_version"`
	Definition *Node  `json:"definition"`
}
type Service struct {
	Name         string       `json:"name"`
	Path         string       `json:"path"`
	Kind         string       `json:"kind"`
	IsService    string       `json:"is_service"`
	Methods      []Member     `json:"methods"`
	Events       []Member     `json:"events"`
	Fields       []Member     `json:"fields"`
	DataElements []Member     `json:"data_elements"`
	Deployments  []Deployment `json:"deployments"`
	Definition   *Node        `json:"definition"`
}
type DataType struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Definition *Node  `json:"definition"`
}
type Warning struct {
	Path      string `json:"path,omitempty"`
	Reference string `json:"reference,omitempty"`
	Message   string `json:"message"`
}
type Document struct {
	Format   string         `json:"format"`
	Source   string         `json:"source"`
	Services []Service      `json:"services"`
	Types    []DataType     `json:"types"`
	Warnings []Warning      `json:"warnings"`
	Summary  map[string]int `json:"summary"`
}

func (d *Document) Service(path string) *Service {
	for i := range d.Services {
		if d.Services[i].Path == path {
			return &d.Services[i]
		}
	}
	return nil
}
