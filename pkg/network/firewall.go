package network

import (
	"sync"

	"github.com/luscis/openlan/pkg/config"
	"github.com/luscis/openlan/pkg/libol"
	"github.com/moby/libnetwork/iptables"
)

const (
	OLCInput   = "OPENLAN_in"
	OLCForward = "OPENLAN_for"
	OLCOutput  = "OPENLAN_out"
	OLCPre     = "OPENLAN_pre"
	OLCPost    = "OPENLAN_pos"
)

type FireWallGlobal struct {
	lock   sync.Mutex
	chains IpChains
	rules  IpRules
}

func NewFireWallGlobal(flows []config.FlowRule) *FireWallGlobal {
	f := &FireWallGlobal{
		chains: make(IpChains, 0, 8),
		rules:  make(IpRules, 0, 32),
	}
	// Load custom rules.
	for _, rule := range flows {
		f.rules = f.rules.Add(IpRule{
			Table:    rule.Table,
			Chain:    rule.Chain,
			Source:   rule.Source,
			Dest:     rule.Dest,
			Jump:     rule.Jump,
			ToSource: rule.ToSource,
			ToDest:   rule.ToDest,
			Comment:  rule.Comment,
			Proto:    rule.Proto,
			Match:    rule.Match,
			DstPort:  rule.DstPort,
			SrcPort:  rule.SrcPort,
			Input:    rule.Input,
			Output:   rule.Output,
		})
	}
	return f
}

func (f *FireWallGlobal) addOLC() {
	f.AddChain(IpChain{Table: TFilter, Name: OLCInput})
	f.AddChain(IpChain{Table: TFilter, Name: OLCForward})
	f.AddChain(IpChain{Table: TFilter, Name: OLCOutput})
	f.AddChain(IpChain{Table: TNat, Name: OLCPre})
	f.AddChain(IpChain{Table: TNat, Name: OLCInput})
	f.AddChain(IpChain{Table: TNat, Name: OLCPost})
	f.AddChain(IpChain{Table: TNat, Name: OLCOutput})
	f.AddChain(IpChain{Table: TMangle, Name: OLCPre})
	f.AddChain(IpChain{Table: TMangle, Name: OLCInput})
	f.AddChain(IpChain{Table: TMangle, Name: OLCForward})
	f.AddChain(IpChain{Table: TMangle, Name: OLCPost})
	f.AddChain(IpChain{Table: TMangle, Name: OLCOutput})
	f.AddChain(IpChain{Table: TRaw, Name: OLCPre})
	f.AddChain(IpChain{Table: TRaw, Name: OLCOutput})
}

func (f *FireWallGlobal) jumpOLC() {
	// Filter Table
	f.AddRule(IpRule{Order: "-I", Table: TFilter, Chain: CInput, Jump: OLCInput})
	f.AddRule(IpRule{Order: "-I", Table: TFilter, Chain: CForward, Jump: OLCForward})
	f.AddRule(IpRule{Order: "-I", Table: TFilter, Chain: COutput, Jump: OLCOutput})

	// NAT Table
	f.AddRule(IpRule{Order: "-I", Table: TNat, Chain: CPre, Jump: OLCPre})
	f.AddRule(IpRule{Order: "-I", Table: TNat, Chain: CInput, Jump: OLCInput})
	f.AddRule(IpRule{Order: "-I", Table: TNat, Chain: CPost, Jump: OLCPost})
	f.AddRule(IpRule{Order: "-I", Table: TNat, Chain: COutput, Jump: OLCOutput})

	// Mangle Table
	f.AddRule(IpRule{Order: "-I", Table: TMangle, Chain: CPre, Jump: OLCPre})
	f.AddRule(IpRule{Order: "-I", Table: TMangle, Chain: CInput, Jump: OLCInput})
	f.AddRule(IpRule{Order: "-I", Table: TMangle, Chain: CForward, Jump: OLCForward})
	f.AddRule(IpRule{Order: "-I", Table: TMangle, Chain: CPost, Jump: OLCPost})
	f.AddRule(IpRule{Order: "-I", Table: TMangle, Chain: COutput, Jump: OLCOutput})

	// Raw Table
	f.AddRule(IpRule{Order: "-I", Table: TRaw, Chain: CPre, Jump: OLCPre})
	f.AddRule(IpRule{Order: "-I", Table: TRaw, Chain: COutput, Jump: OLCOutput})
}

func (f *FireWallGlobal) Initialize() {
	IptableInit()
	// Init chains
	f.addOLC()
	f.jumpOLC()
}

func (f *FireWallGlobal) AddChain(chain IpChain) {
	f.chains = f.chains.Add(chain)
}

func (f *FireWallGlobal) AddRule(rule IpRule) {
	f.rules = f.rules.Add(rule)
}

func (f *FireWallGlobal) InstallRule(rule IpRule) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	order := rule.Order
	if order == "" {
		order = "-A"
	}
	if _, err := rule.Opr(order); err != nil {
		return err
	}
	f.rules = f.rules.Add(rule)
	return nil
}

func (f *FireWallGlobal) install() {
	for _, c := range f.chains {
		if _, err := c.Opr("-N"); err != nil {
			libol.Error("FireWall.install %s", err)
		}
	}
	for _, r := range f.rules {
		order := r.Order
		if order == "" {
			order = "-A"
		}
		if _, err := r.Opr(order); err != nil {
			libol.Error("FireWall.install %s", err)
		}
	}
}

func (f *FireWallGlobal) Start() {
	f.lock.Lock()
	defer f.lock.Unlock()
	libol.Info("FireWall.Start")
	f.install()
	iptables.OnReloaded(func() {
		libol.Info("FireWall.Start OnReloaded")
		f.lock.Lock()
		defer f.lock.Unlock()
		f.install()
	})
}

func (f *FireWallGlobal) cancel() {
	for _, r := range f.rules {
		if _, err := r.Opr("-D"); err != nil {
			libol.Warn("FireWall.cancel %s", err)
		}
	}
	for _, c := range f.chains {
		if _, err := c.Opr("-X"); err != nil {
			libol.Warn("FireWall.cancel %s", err)
		}
	}
}

func (f *FireWallGlobal) CancelRule(rule IpRule) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, err := rule.Opr("-D"); err != nil {
		return err
	}
	f.rules = f.rules.Remove(rule)
	return nil
}

func (f *FireWallGlobal) Stop() {
	f.lock.Lock()
	defer f.lock.Unlock()
	libol.Info("FireWall.Stop")
	f.cancel()
}

func (f *FireWallGlobal) Refresh() {
	f.cancel()
	f.install()
}

type FireWallJump struct {
	rules IpRules
}

func (j *FireWallJump) Install(ch IpChain) {
	r := IpRule{
		Order: "-I",
		Table: ch.Table,
		Chain: ch.From,
		Jump:  ch.Name,
	}

	if _, err := r.Opr(r.Order); err != nil {
		libol.Error("FireWallJump.install %s", err)
		return
	}

	j.rules = j.rules.Add(r)
}

func (j *FireWallJump) Cancel() {
	for _, r := range j.rules {
		if _, err := r.Opr("-D"); err != nil {
			libol.Warn("FireWallJump.cancel %s", err)
		}
	}
}

type FireWallChain struct {
	name   string
	parent string
	rules  IpRules
	table  string
}

func NewFireWallChain(name, table, parent string) *FireWallChain {
	return &FireWallChain{
		name:   name,
		table:  table,
		parent: parent,
	}
}

func (ch *FireWallChain) new() {
	c := ch.Chain()
	if _, err := c.Opr("-N"); err != nil {
		libol.Error("FireWallChain.new %s", err)
	}
}

func (ch *FireWallChain) free() {
	c := ch.Chain()
	if _, err := c.Opr("-X"); err != nil {
		libol.Error("FireWallChain.free %s", err)
	}
}

func (ch *FireWallChain) Chain() IpChain {
	return IpChain{
		Table: ch.table,
		Name:  ch.parent + "-" + ch.name,
		From:  ch.parent,
	}
}

func (ch *FireWallChain) AddRule(rule IpRule) {
	chain := ch.Chain()
	rule.Table = chain.Table
	rule.Chain = chain.Name
	ch.rules = ch.rules.Add(rule)
}

func (ch *FireWallChain) Install() {
	ch.new()
	for _, r := range ch.rules {
		order := r.Order
		if order == "" {
			order = "-A"
		}
		if _, err := r.Opr(order); err != nil {
			libol.Error("FireWallChain.install %s", err)
		}
	}
}

func (ch *FireWallChain) Cancel() {
	for _, c := range ch.rules {
		if _, err := c.Opr("-D"); err != nil {
			libol.Warn("FireWall.cancel %s", err)
		}
	}
	ch.free()
}

type FireWallFilter struct {
	name string
	In   *FireWallChain
	Out  *FireWallChain
	For  *FireWallChain
	Jump *FireWallJump
}

func NewFireWallFilter(name string) *FireWallFilter {
	return &FireWallFilter{
		In:   NewFireWallChain(name, TFilter, OLCInput),
		For:  NewFireWallChain(name, TFilter, OLCForward),
		Out:  NewFireWallChain(name, TFilter, OLCOutput),
		Jump: &FireWallJump{},
	}
}

func (f *FireWallFilter) Install() {
	// Install Chain Rules
	f.In.Install()
	f.For.Install()
	f.Out.Install()

	// Add Jump Rules
	f.Jump.Install(f.In.Chain())
	f.Jump.Install(f.For.Chain())
	f.Jump.Install(f.Out.Chain())
}

func (f *FireWallFilter) Cancel() {
	// Remove Jump Rules
	f.Jump.Cancel()
	// Cancel Chain Rules
	f.In.Cancel()
	f.For.Cancel()
	f.Out.Cancel()
}

type FireWallNATPre struct {
	*FireWallChain
}

func (ch *FireWallNATPre) Chain() IpChain {
	return IpChain{
		Table: TNat,
		Name:  OLCPre + "-" + ch.name,
		From:  ch.parent,
	}
}

type FireWallNAT struct {
	name string
	Pre  *FireWallChain
	In   *FireWallChain
	Out  *FireWallChain
	Post *FireWallChain
	Jump *FireWallJump
}

func NewFireWallNAT(name string) *FireWallNAT {
	return &FireWallNAT{
		Pre:  NewFireWallChain(name, TNat, OLCPre),
		In:   NewFireWallChain(name, TNat, OLCInput),
		Out:  NewFireWallChain(name, TNat, OLCOutput),
		Post: NewFireWallChain(name, TNat, OLCPost),
		Jump: &FireWallJump{},
	}
}

func (n *FireWallNAT) Install() {
	// Install Chain Rules
	n.Pre.Install()
	n.In.Install()
	n.Out.Install()
	n.Post.Install()

	// Add Jump Rules
	n.Jump.Install(n.Pre.Chain())
	n.Jump.Install(n.In.Chain())
	n.Jump.Install(n.Out.Chain())
	n.Jump.Install(n.Post.Chain())
}

func (n *FireWallNAT) Cancel() {
	// Remove Jump Rules
	n.Jump.Cancel()
	// Cancel Chain Rules
	n.Pre.Cancel()
	n.In.Cancel()
	n.Out.Cancel()
	n.Post.Cancel()
}

type FireWallMangle struct {
	name string
	Pre  *FireWallChain
	In   *FireWallChain
	For  *FireWallChain
	Out  *FireWallChain
	Post *FireWallChain
	Jump *FireWallJump
}

func NewFireWallMangle(name string) *FireWallMangle {
	return &FireWallMangle{
		Pre:  NewFireWallChain(name, TMangle, OLCPre),
		In:   NewFireWallChain(name, TMangle, OLCInput),
		For:  NewFireWallChain(name, TMangle, OLCForward),
		Out:  NewFireWallChain(name, TMangle, OLCOutput),
		Post: NewFireWallChain(name, TMangle, OLCPost),
		Jump: &FireWallJump{},
	}
}

func (m *FireWallMangle) Install() {
	// Install Chain Rules
	m.Pre.Install()
	m.In.Install()
	m.For.Install()
	m.Out.Install()
	m.Post.Install()

	// Add Jump Rules
	m.Jump.Install(m.Pre.Chain())
	m.Jump.Install(m.In.Chain())
	m.Jump.Install(m.For.Chain())
	m.Jump.Install(m.Out.Chain())
	m.Jump.Install(m.Post.Chain())
}

func (m *FireWallMangle) Cancel() {
	// Remove Jump Rules
	m.Jump.Cancel()
	// Cancel Chain Rules
	m.Pre.Cancel()
	m.In.Cancel()
	m.For.Cancel()
	m.Out.Cancel()
	m.Post.Cancel()
}

type FireWallRaw struct {
	name string
	Pre  *FireWallChain
	Out  *FireWallChain
	Jump *FireWallJump
}

func NewFireWallRaw(name string) *FireWallRaw {
	return &FireWallRaw{
		Pre:  NewFireWallChain(name, TRaw, OLCPre),
		Out:  NewFireWallChain(name, TRaw, OLCOutput),
		Jump: &FireWallJump{},
	}
}
func (r *FireWallRaw) Install() {
	// Install Chain Rules
	r.Pre.Install()
	r.Out.Install()

	// Add Jump Rules
	r.Jump.Install(r.Pre.Chain())
	r.Jump.Install(r.Out.Chain())
}

func (r *FireWallRaw) Cancel() {
	// Remove Jump Rules
	r.Jump.Cancel()
	// Cancel Chain Rules
	r.Pre.Cancel()
	r.Out.Cancel()
}

type FireWallTable struct {
	Filter *FireWallFilter
	Nat    *FireWallNAT
	Mangle *FireWallMangle
	Raw    *FireWallRaw
}

func NewFireWallTable(name string) *FireWallTable {
	IptableInit()
	return &FireWallTable{
		Filter: NewFireWallFilter(name),
		Nat:    NewFireWallNAT(name),
		Mangle: NewFireWallMangle(name),
		Raw:    NewFireWallRaw(name),
	}
}

func (t *FireWallTable) Start() {
	t.Filter.Install()
	t.Nat.Install()
	t.Mangle.Install()
	t.Raw.Install()
}

func (t *FireWallTable) Stop() {
	t.Raw.Cancel()
	t.Mangle.Cancel()
	t.Nat.Cancel()
	t.Filter.Cancel()
}
