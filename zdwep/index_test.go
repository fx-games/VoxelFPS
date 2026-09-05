package zdwep

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func indexFromFiles(t *testing.T, files map[string]string) *Index {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	src, err := NewSourceFS(fsys)
	require.NoError(t, err)
	idx, err := BuildIndex(src)
	require.NoError(t, err)
	return idx
}

func TestIndexFollowsIncludesCaseInsensitively(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{
		"DECORATE.txt": "#include \"actors/gun.txt\"\n",
		"Actors/Gun.TXT": `
ACTOR TestGun : Weapon
{
  Tag "Test Gun"
}
`,
	})

	actor, ok := idx.Actor("testgun")
	require.True(t, ok)
	assert.Equal(t, "TestGun", actor.Name)
	assert.Equal(t, "Weapon", actor.Parent)

	tag, ok := actor.Property("Tag")
	require.True(t, ok)
	assert.Equal(t, `"Test Gun"`, tag)
}

func TestIndexResolvesInheritanceThroughUndefinedParents(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{
		"DECORATE": `
ACTOR ModWeapon : Weapon {}
ACTOR ModShotgun : ModWeapon {}
ACTOR ModShells : Ammo {}
`,
	})

	// Weapon itself is an engine class the mod never defines, so the chain has
	// to reach it by name rather than by finding an actor.
	assert.True(t, idx.InheritsFrom("ModShotgun", "Weapon"))
	assert.True(t, idx.InheritsFrom("ModShotgun", "ModWeapon"))
	assert.False(t, idx.InheritsFrom("ModShells", "Weapon"))
	assert.True(t, idx.InheritsFrom("ModShells", "Ammo"))

	chain := idx.Ancestry("ModShotgun")
	require.Len(t, chain, 2)
	assert.Equal(t, "ModShotgun", chain[0].Name)
	assert.Equal(t, "ModWeapon", chain[1].Name)
}

func TestIndexKeepsStateFallthroughBetweenLabels(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{
		"DECORATE": `
ACTOR Faller : Weapon
{
  States
  {
  RefireE:
    HSHT D 1
  RefireD:
    HSHT C 1
    Goto Fire
  Fire:
    HSHT A 2
    Loop
  }
}
`,
	})

	actor, ok := idx.Actor("faller")
	require.True(t, ok)

	// One flat list, labels pointing into it: RefireE runs on into RefireD,
	// which is the whole point of writing it that way.
	require.Len(t, actor.States.Lines, 5)

	refireE, ok := actor.States.Label("RefireE")
	require.True(t, ok)
	assert.Equal(t, 0, refireE)

	refireD, ok := actor.States.Label("refired")
	require.True(t, ok)
	assert.Equal(t, 1, refireD)

	fire, ok := actor.States.Label("Fire")
	require.True(t, ok)
	assert.Equal(t, 3, fire)
}

func TestIndexParsesStateLineDetails(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{
		"DECORATE": `
ACTOR Detailed : Weapon
{
  +NOGRAVITY
  -SOLID
  Scale 0.6
  States
  {
  Spawn:
    HSHT N 1 Bright A_SetPitch (pitch - 0.8)
    HSHT AB random(2, 5) Offset(3, -4) A_FireBullets (4, 0.5, 10, 5, "FDBulletPuff", 0)
    "####" "#" 0 A_Raise
    TNT1 A -1
    Goto Super::Ready+2
  }
}
`,
	})

	actor, ok := idx.Actor("detailed")
	require.True(t, ok)
	assert.True(t, actor.Flags["nogravity"])
	assert.False(t, actor.Flags["solid"])

	scale, ok := actor.Property("scale")
	require.True(t, ok)
	assert.Equal(t, "0.6", scale)

	lines := actor.States.Lines
	require.Len(t, lines, 5)

	assert.Equal(t, "HSHT", lines[0].Sprite)
	assert.Equal(t, "N", lines[0].Frames)
	assert.Equal(t, 1, lines[0].Duration)
	assert.True(t, lines[0].Bright)
	assert.Equal(t, "A_SetPitch (pitch - 0.8)", lines[0].Action)

	assert.Equal(t, "AB", lines[1].Frames)
	assert.Equal(t, 2, lines[1].Duration)
	assert.Equal(t, 5, lines[1].DurationMax)
	assert.Equal(t, 3, lines[1].OffsetX)
	assert.Equal(t, -4, lines[1].OffsetY)
	assert.Equal(t, `A_FireBullets (4, 0.5, 10, 5, "FDBulletPuff", 0)`, lines[1].Action)

	assert.Equal(t, "####", lines[2].Sprite)
	assert.Equal(t, "#", lines[2].Frames)
	assert.Equal(t, "A_Raise", lines[2].Action)

	assert.Equal(t, -1, lines[3].Duration)

	assert.Equal(t, "goto", lines[4].Flow)
	assert.Equal(t, "Super::Ready", lines[4].FlowTarget)
	assert.Equal(t, 2, lines[4].FlowOffset)
}

func TestIndexIgnoresComments(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{
		"DECORATE": `
/* a block comment
   spanning lines */
ACTOR Commented : Weapon // trailing
{
  // a whole line
  Tag "Commented"
  States
  {
  Ready:
    HSHT A 1 A_WeaponReady // and here
    Loop
  }
}
`,
	})

	actor, ok := idx.Actor("commented")
	require.True(t, ok)
	require.Len(t, actor.States.Lines, 2)
	assert.Equal(t, "A_WeaponReady", actor.States.Lines[0].Action)
	// The block comment is replaced by blank lines, not removed, so reported
	// line numbers still match the file a human opens.
	assert.Equal(t, 11, actor.States.Lines[0].Line)
}
