package quads

import (
    "fmt"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/memmaker/goray/quads/glhf"
)

type GuiOption interface {
    String() string
    More()
    Less()
    Activate()
}
type GuiConfig struct {
    *GuiList
    options []GuiOption
}

type GuiOptionList struct {
    name        string
    value       *int
    options     []string
    changeIndex func(index int)
}

func (g *GuiOptionList) String() string {
    return fmt.Sprintf("%s: %s", g.name, g.options[*g.value])
}

func (g *GuiOptionList) More() {
    *g.value++
    if *g.value >= len(g.options) {
        *g.value = 0
    }
}

func (g *GuiOptionList) Less() {
    *g.value--
    if *g.value < 0 {
        *g.value = len(g.options) - 1
    }
}

func (g *GuiOptionList) Activate() {
    g.changeIndex(*g.value)
}

func NewGuiOptionList(label string, options []string, currentIndex *int, changeIndex func(index int)) *GuiOptionList {
    return &GuiOptionList{
        name:        label,
        value:       currentIndex,
        options:     options,
        changeIndex: changeIndex,
    }
}

type GuiOptionAction struct {
    name   string
    action func()
}

func (g GuiOptionAction) String() string {
    return g.name
}

func (g GuiOptionAction) More() {

}

func (g GuiOptionAction) Less() {

}

func (g GuiOptionAction) Activate() {
    g.action()
}

func NewGuiOptionAction(name string, action func()) *GuiOptionAction {
    return &GuiOptionAction{
        name:   name,
        action: action,
    }
}

type GuiOptionBool struct {
    name  string
    value *bool
}

func NewGuiOptionBool(name string, value *bool) *GuiOptionBool {
    return &GuiOptionBool{
        name:  name,
        value: value,
    }
}

func (o *GuiOptionBool) String() string {
    if *o.value {
        return fmt.Sprintf("%s: true", o.name)
    }
    return fmt.Sprintf("%s: false", o.name)
}

func (o *GuiOptionBool) More() {
    *o.value = true
}

func (o *GuiOptionBool) Less() {
    *o.value = false
}

func (o *GuiOptionBool) Activate() {
    *o.value = !*o.value
}

type GuiOptionFloat struct {
    name     string
    value    *float32
    stepSize float32
    activate func()
}

func NewGuiOptionFloat(name string, value *float32) *GuiOptionFloat {
    return &GuiOptionFloat{
        name:     name,
        value:    value,
        stepSize: 0.1,
    }
}

func (o *GuiOptionFloat) StepSize(stepSize float32) *GuiOptionFloat {
    o.stepSize = stepSize
    return o
}

func (o *GuiOptionFloat) ActivateFunc(activate func()) *GuiOptionFloat {
    o.activate = activate
    return o
}

func (o *GuiOptionFloat) String() string {
    return fmt.Sprintf("%s: %.2f", o.name, *o.value)
}

func (o *GuiOptionFloat) More() {
    *o.value += o.stepSize
}

func (o *GuiOptionFloat) Less() {
    *o.value -= o.stepSize
}

func (o *GuiOptionFloat) Activate() {
    if o.activate != nil {
        o.activate()
    }
}

func NewGuiConfig(shader *glhf.Shader, uiTexture *glhf.Texture, font BitmapFont) *GuiConfig {
    g := &GuiConfig{
        GuiList: NewGuiList(shader, uiTexture, font),
    }
    g.SetKeyHandler(g.handleKeys)
    return g
}
func (g *GuiConfig) SetCentered(screenWidth int, screenHeight int, options []GuiOption) {
    g.options = options
    labels := make([]string, len(g.options))
    for i, option := range g.options {
        labels[i] = option.String()
    }
    g.SetBounds(float32(screenWidth)*0.7, float32(screenHeight)*0.8)
    g.SetOrigin(float32(screenWidth/2), float32(screenHeight/2))
    g.SetCenteredOnScreen()
    g.SetItems(labels)
    g.layout()
}

func (g *GuiConfig) handleKeys(index int, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
    if action != glfw.Press {
        return false
    }
    selected := g.GetSelectedIndex()
    if selected < 0 || selected >= len(g.options) {
        return false
    }
    selectedOption := g.options[selected]
    switch key {
    case glfw.KeyE:
        selectedOption.Activate()
        g.ReplaceText(selected, selectedOption.String())
        return true
    case glfw.KeyD:
        selectedOption.More()
        g.ReplaceText(selected, selectedOption.String())
        return true
    case glfw.KeyA:
        selectedOption.Less()
        g.ReplaceText(selected, selectedOption.String())
        return true
    case glfw.KeyW:
        g.SelectPrevious()
        return true
    case glfw.KeyS:
        g.SelectNext()
        return true
    case glfw.KeyEscape:
        g.Close()
        return true
    }
    return false
}
