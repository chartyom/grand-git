package gg

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type layoutNode struct {
	w, h, x, y, pane int
	kind             byte
	children         []*layoutNode
}
type layoutParser struct {
	s   string
	pos int
}

func (p *layoutParser) number() (int, error) {
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, errors.New("invalid tmux layout")
	}
	return strconv.Atoi(p.s[start:p.pos])
}
func (p *layoutParser) expect(c byte) error {
	if p.pos >= len(p.s) || p.s[p.pos] != c {
		return errors.New("invalid tmux layout")
	}
	p.pos++
	return nil
}
func (p *layoutParser) node() (*layoutNode, error) {
	n := &layoutNode{}
	var err error
	if n.w, err = p.number(); err != nil {
		return nil, err
	}
	if err = p.expect('x'); err != nil {
		return nil, err
	}
	if n.h, err = p.number(); err != nil {
		return nil, err
	}
	if err = p.expect(','); err != nil {
		return nil, err
	}
	if n.x, err = p.number(); err != nil {
		return nil, err
	}
	if err = p.expect(','); err != nil {
		return nil, err
	}
	if n.y, err = p.number(); err != nil {
		return nil, err
	}
	if p.pos < len(p.s) && (p.s[p.pos] == '{' || p.s[p.pos] == '[') {
		n.kind = p.s[p.pos]
		p.pos++
		end := byte('}')
		if n.kind == '[' {
			end = ']'
		}
		for {
			child, e := p.node()
			if e != nil {
				return nil, e
			}
			n.children = append(n.children, child)
			if p.pos < len(p.s) && p.s[p.pos] == end {
				p.pos++
				break
			}
			if e = p.expect(','); e != nil {
				return nil, e
			}
		}
	} else {
		if err = p.expect(','); err != nil {
			return nil, err
		}
		if n.pane, err = p.number(); err != nil {
			return nil, err
		}
	}
	return n, nil
}
func (n *layoutNode) fit(w, h, x, y int) {
	oldW, oldH := n.w, n.h
	n.w, n.h, n.x, n.y = w, h, x, y
	if len(n.children) == 0 {
		return
	}
	horizontal := n.kind == '{'
	space := h
	old := oldH
	if horizontal {
		space = w
		old = oldW
	}
	remaining := space - len(n.children) + 1
	old -= len(n.children) - 1
	offset := 0
	for i, c := range n.children {
		weight := c.h
		if horizontal {
			weight = c.w
		}
		size := remaining
		if i < len(n.children)-1 && old > 0 {
			size = int(float64(space-len(n.children)+1) * float64(weight) / float64(old))
			size = max(2, min(size, remaining-2*(len(n.children)-i-1)))
		}
		if horizontal {
			c.fit(size, h, x+offset, y)
		} else {
			c.fit(w, size, x, y+offset)
		}
		offset += size + 1
		remaining -= size
	}
}
func (n *layoutNode) remove(pane int) *layoutNode {
	if len(n.children) == 0 {
		if n.pane == pane {
			return nil
		}
		return n
	}
	out := []*layoutNode{}
	for _, c := range n.children {
		if c = c.remove(pane); c != nil {
			out = append(out, c)
		}
	}
	n.children = out
	if len(out) == 0 {
		return nil
	}
	if len(out) == 1 {
		out[0].fit(n.w, n.h, n.x, n.y)
		return out[0]
	}
	return n
}
func (n *layoutNode) String() string {
	head := fmt.Sprintf("%dx%d,%d,%d", n.w, n.h, n.x, n.y)
	if len(n.children) == 0 {
		return head + "," + strconv.Itoa(n.pane)
	}
	end := "}"
	if n.kind == '[' {
		end = "]"
	}
	children := []string{}
	for _, c := range n.children {
		children = append(children, c.String())
	}
	return head + string(n.kind) + strings.Join(children, ",") + end
}
func layoutChecksum(s string) uint16 {
	var sum uint16
	for _, b := range []byte(s) {
		sum = (sum >> 1) | ((sum & 1) << 15)
		sum += uint16(b)
	}
	return sum
}
func (p *Project) fullHeightSidebar(side, window string) error {
	raw, err := p.tmux("display-message", "-p", "-t", window, "#{window_layout}")
	if err != nil {
		return err
	}
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return errors.New("invalid tmux layout")
	}
	parser := layoutParser{s: raw[comma+1:]}
	tree, err := parser.node()
	if err != nil {
		return err
	}
	sideID, err := strconv.Atoi(strings.TrimPrefix(side, "%"))
	if err != nil {
		return err
	}
	w, h := tree.w, tree.h
	terms := tree.remove(sideID)
	if terms == nil {
		return errors.New("terminal layout missing")
	}
	sw := sidebarWidth(w)
	terms.fit(w-sw-1, h, sw+1, 0)
	root := &layoutNode{w: w, h: h, kind: '{', children: []*layoutNode{{w: sw, h: h, pane: sideID}, terms}}
	layout := root.String()
	_, err = p.tmux("select-layout", "-t", window, fmt.Sprintf("%04x,%s", layoutChecksum(layout), layout))
	return err
}
