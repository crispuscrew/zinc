package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/creator/internal/keys"
)

type formResult int

const (
	formStay formResult = iota
	formSave
	formCancel
	formEdit
	formResolve
)

type fieldKind int

const (
	kindText fieldKind = iota
	kindMultiline
	kindBool
	kindInfo
	kindAction
	kindEnum
)

type formField struct {
	label   string
	kind    fieldKind
	input   *textinput.Model
	area    *textarea.Model
	get     func() string
	set     func(string)
	bget    func() bool
	bset    func(bool)
	info    func() string
	values  []string
	rebuild bool
}

type configBinding struct {
	label, initial string
	value          func() string
	apply          func(*schema.AppConfig, string) error
}

type formModel struct {
	original                                           schema.AppConfig
	originalVM                                         *vmoptions.Config
	creating                                           bool
	draft                                              schema.AppConfig
	scheme                                             keys.Scheme
	name, image, entrypoint, desc, icon, group         textinput.Model
	dbusTalk, dbusOwn                                  textinput.Model
	baseDigest, memory, vcpus, diskSize, ciUser, ciKey textinput.Model
	install                                            textarea.Model
	vmMedia, vmForwards                                textarea.Model
	vm                                                 vmoptions.Config
	bindings                                           []configBinding
	common, virtual, fields                            []formField
	idx, height                                        int
	err                                                error
}

func newForm(base schema.AppConfig, creating bool) *formModel {
	frm := &formModel{creating: creating, original: base, vm: vmoptions.Default(base.AppNameID, base.ImageMeta.Image)}
	frm.reload(base)
	return frm
}

// Rebind every control after an external edit. No decoded maps or lists are mutated
// in place: changed structured controls replace their value, preserving cancelled drafts.
func (frm *formModel) reload(cfg schema.AppConfig) {
	if !frm.creating && frm.draft.AppNameID != "" && cfg.AppNameID != frm.draft.AppNameID {
		frm.err = fmt.Errorf("the editor cannot rename an app; use rename from the list")
		return
	}
	frm.draft = cfg
	if frm.draft.SchemaVersion == 0 {
		frm.draft.SchemaVersion = schema.SchemaVersion
	}
	if frm.draft.Type == "" {
		frm.draft.Type = schema.ZincContainer
	}
	frm.bindings = nil
	frm.common = frm.sharedFields()
	frm.virtual = frm.vmFields()
	frm.buildFields()
	frm.idx, frm.err = -1, nil
	frm.focusNext()
}

func (frm *formModel) loadVM(options vmoptions.Config) {
	baseline := options
	frm.originalVM = &baseline
	frm.vm = options
	frm.virtual = frm.vmFields()
	frm.buildFields()
}

func (frm *formModel) buildFields() {
	frm.fields = append([]formField(nil), frm.common...)
	if frm.draft.Type == schema.ZincVirtualization {
		frm.fields = append(frm.fields, frm.virtual...)
	}
	frm.fields = append(frm.fields, formField{label: "advanced", kind: kindAction, info: frm.advancedSummary})
}

func newInput(value, placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt, input.Placeholder, input.CharLimit = "", placeholder, 0
	input.SetValue(value)
	return input
}

func newArea(value, placeholder string) textarea.Model {
	area := textarea.New()
	area.Prompt, area.Placeholder, area.CharLimit = "", placeholder, 0
	area.ShowLineNumbers = false
	area.SetWidth(64)
	area.SetHeight(3)
	area.SetValue(value)
	area.Blur()
	return area
}
