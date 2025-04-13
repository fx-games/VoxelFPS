package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/solarlune/routine"
    "github.com/solarlune/routine/actions"
    "time"
)

type GuiToast struct {
    label     *BitmapFontMesh
    border    *GuiBorder
    guiShader *glhf.Shader
    routine   *routine.Routine
}

func NewGuiToast(uiContext UIContext, label *BitmapFontMesh) *GuiToast {
    size, pos := label.GetRect()
    border := NewGuiBorderWithBounds(uiContext, size, pos)
    border.SetBackground(mgl32.Vec4{0, 0, 0, 0.8})
    animRoutine := routine.New()
    labelAlpha := label.GetAlpha()
    label.SetAlpha(0)
    border.SetAlpha(0)

    animRoutine.Define(0,
        actions.NewFunction(func(block *routine.Block) routine.Flow {
            curAlpha := label.GetAlpha()
            if curAlpha < labelAlpha {
                curAlpha += 0.1
                label.SetAlpha(curAlpha)
                border.SetAlpha(curAlpha)
                return routine.FlowIdle
            }

            return routine.FlowNext
        }),

        actions.NewWait(2*time.Second),

        actions.NewFunction(func(block *routine.Block) routine.Flow {
            curAlpha := label.GetAlpha()
            if curAlpha > 0 {
                curAlpha -= 0.1
                label.SetAlpha(curAlpha)
                border.SetAlpha(curAlpha)
                return routine.FlowIdle
            }
            label.Hide()
            return routine.FlowNext
        }),

        actions.NewWait(250*time.Millisecond),
    )

    animRoutine.Run(0)

    return &GuiToast{
        guiShader: uiContext.Shader,
        label:     label,
        routine:   animRoutine,
        border:    border,
    }
}

func (g *GuiToast) Update(dt float64) {
    if !g.routine.Running() {
        return
    }
    g.routine.Update()
}

func (g *GuiToast) ShouldClose() bool {
    return !g.routine.Running()
}

func (g *GuiToast) Draw() {
    g.border.Draw()
    g.label.Draw()
}
