package zdwep

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

/*
Core Weapon ParsedScript:
Spawn: Defines how the gun looks as a pickup
Ready: Entered when the weapon is selected, will signal readiness to fire and deselect
Select: Animation to raise the weapon
Deselect: Animation to lower the weapon
Fire: The main firing state
Flash: The muzzle flash, entered at the same time as Fire
*/

type WeaponScriptLabel struct {
	Label  string
	States []WeaponState
}

type DoomWeapon struct {
	Name           string
	OriginalScript string
	ParsedScript   []WeaponScriptLabel
	SourceFile     string
	MissingSprites []string
	MissingSounds  []string
	FoundSprites   []string
	FoundSounds    []string
	NeededSprites  []string
	NeededSounds   []string
	ReadySound     string
	UpSound        string
	PickupSound    string
	YAdjust        int
}

func (w DoomWeapon) String() string {
	assetSuffix := fmt.Sprintf(" (%d/%d sprites, %d/%d sounds)", len(w.FoundSprites), len(w.NeededSprites), len(w.FoundSounds), len(w.NeededSounds))

	return fmt.Sprintf("%s%s", w.GetName(), assetSuffix)
}

func (w DoomWeapon) GetName() string {
	if w.Name != "" {
		return w.Name
	}
	return fmt.Sprintf("no name: %s", w.SourceFile)
}

func (w DoomWeapon) IsComplete() bool {
	return len(w.MissingSprites) == 0 && len(w.MissingSounds) == 0
}

type WeaponState struct {
	Sprite              string
	Frame               string
	DurationInTics      int // DurationInTics, 1 Tic = 1/35 seconds
	KeywordsAndFunction string
	ControlFlow         string
}

func (ws WeaponState) GetSpriteNames() []string {
	var frames []string
	if ws.DurationInTics == 0 || strings.ContainsRune(ws.Frame, '#') || ws.Sprite == "----" || ws.Sprite == "####" {
		return frames
	}
	for _, c := range ws.Frame {
		frames = append(frames, fmt.Sprintf("%s%c0", ws.Sprite, c))
	}
	return frames
}

func (ws WeaponState) HasSprite() bool {
	// TNT1 means invisible sprite in zscript
	// NULLA0 is also a special case of invisible sprite
	return ws.ControlFlow == "" && ws.Frame != "####" && ws.Sprite != "" && strings.ToLower(ws.Sprite) != "tnt1" && strings.ToLower(ws.Sprite) != "null"
}

var soundDefRegex = regexp.MustCompile(`A_PlaySound\("([^"]+)".*\)`)
var weaponSoundDefRegex = regexp.MustCompile(`A_PlayWeaponSound\("([^"]+)".*\)`)

func (ws WeaponState) HasSound() bool {
	return weaponSoundDefRegex.MatchString(ws.KeywordsAndFunction) || soundDefRegex.MatchString(ws.KeywordsAndFunction)
}

func (ws WeaponState) GetSoundFilename() string {
	submatch := soundDefRegex.FindStringSubmatch(ws.KeywordsAndFunction)
	if len(submatch) >= 2 {
		return submatch[1]
	}

	submatch = weaponSoundDefRegex.FindStringSubmatch(ws.KeywordsAndFunction)

	if len(submatch) >= 2 {
		return submatch[1]
	}

	return ""
}

func (ws WeaponState) String() string {
	if ws.ControlFlow != "" {
		return fmt.Sprintf("\t%s\n", ws.ControlFlow)
	}
	return fmt.Sprintf("\t%s %s %d %s\n", ws.Sprite, ws.Frame, ws.DurationInTics, ws.KeywordsAndFunction)
}

func readFileFromZip(f *zip.File) []byte {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	all, err := io.ReadAll(rc)

	if err != nil {
		return nil
	}
	return all
}

func sortedListFromMap(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func padRight(str string, length int) string {
	return str + strings.Repeat(" ", length-len(str))
}

type CIZipWrapper struct {
	*zip.ReadCloser
	directory map[string]*zip.File
}

func NewCIZipWrapper(zipFilename string) *CIZipWrapper {
	zf, err := zip.OpenReader(zipFilename)
	if err != nil {
		panic(err)
	}
	directory := make(map[string]*zip.File)
	for _, f := range zf.File {
		directory[strings.ToLower(f.Name)] = f
	}
	return &CIZipWrapper{
		ReadCloser: zf,
		directory:  directory,
	}
}

func (cizw *CIZipWrapper) Open(name string) (fs.File, error) {
	if f, ok := cizw.directory[strings.ToLower(name)]; ok {
		return cizw.ReadCloser.Open(f.Name)
	}

	return nil, errors.New("file not found")
}

func (cizw *CIZipWrapper) Close() error {
	return cizw.ReadCloser.Close()
}

func loadWeaponsFromPk3(pk3file string) []DoomWeapon {
	// Load weapon from PK3 file

	zf := NewCIZipWrapper(pk3file)

	sndInfoMap := readSndInfo(zf)

	resolveSoundName := func(soundName string) string {
		if substName, ok := sndInfoMap[soundName]; ok {
			return substName
		}
		return soundName
	}

	spriteIndex := make(map[string][]string)
	soundIndex := make(map[string][]string)

	for _, f := range zf.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(strings.ToLower(f.Name), "sprites/") {
			baseNameWithoutExt := strings.ToLower(strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name)))
			//directory := path.Dir(f.Name)
			spriteIndex[baseNameWithoutExt] = append(spriteIndex[baseNameWithoutExt], f.Name)
		}
		if strings.HasPrefix(strings.ToLower(f.Name), "sounds/") {
			baseNameWithoutExt := strings.ToLower(strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name)))
			soundIndex[baseNameWithoutExt] = append(soundIndex[baseNameWithoutExt], f.Name)
		}
	}

	println(fmt.Sprintf("Built spriteIndex (%d entries), soundIndex (%d entries) & sndInfoMap (%d entries)", len(spriteIndex), len(soundIndex), len(sndInfoMap)))

	lookupSprite := func(spriteName string) []string {
		return spriteIndex[strings.ToLower(spriteName)]
	}

	lookupSound := func(soundName string) []string {
		resolved := resolveSoundName(soundName)
		base := path.Base(resolved)
		return soundIndex[strings.ToLower(base)]
	}

	var weapons []DoomWeapon
	completeCount := 0
	missingCount := 0

	decorate, decoErr := zf.Open("DECORATE")
	defer decorate.Close()
	isDecorate := decoErr == nil

	if isDecorate {
		decorateFiles := readDecorateFiles(decorate)
		for _, fName := range decorateFiles {
			f, zipErr := zf.Open(fName)
			if zipErr != nil {
				println(zipErr.Error())
				continue
			}
			asString, _ := io.ReadAll(f)
			doomWeapon, err2 := tryParseActor(string(asString), lookupSprite, lookupSound)
			if err2 != nil {
				//println(err2.Error())
				continue
			}
			doomWeapon.SourceFile = fName
			if doomWeapon.Name == "" {
				doomWeapon.Name = path.Base(fName)
			}
			weapons = append(weapons, doomWeapon)

			if doomWeapon.IsComplete() {
				completeCount++
			} else {
				missingCount++
			}
		}
	} else {
		for _, f := range zf.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if strings.HasPrefix(strings.ToLower(f.Name), "actors/") ||
				strings.HasPrefix(strings.ToLower(f.Name), "decorate/") ||
				strings.HasSuffix(strings.ToLower(f.Name), ".txt") {
				doomWeapon, err2 := tryParseActor(string(readFileFromZip(f)), lookupSprite, lookupSound)
				if err2 != nil {
					//println("Could not parse actor", f.Name, err2.Error())
					//println(fmt.Sprintf("..%s (%s)", err2.Error(), f.Name))
					continue
				}
				doomWeapon.SourceFile = f.Name
				weapons = append(weapons, doomWeapon)

				if doomWeapon.IsComplete() {
					completeCount++
				} else {
					missingCount++
				}
			}
			//println(f.Name)
		}
	}

	println(fmt.Sprintf("*** %d weapons found (%d complete, %d missing)", len(weapons), completeCount, missingCount))
	return weapons
}

func readDecorateFiles(decorate fs.File) []string {
	includeRegex := regexp.MustCompile(`#include\s+"?([^"]+)"?`)
	scanner := bufio.NewScanner(decorate)
	var files []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if includeRegex.MatchString(line) {
			submatch := includeRegex.FindStringSubmatch(line)
			files = append(files, submatch[1])
		}
	}
	return files

}

func parseSndInfo(file fs.File) map[string]string {
	// line by line
	scanner := bufio.NewScanner(file)
	soundMap := make(map[string]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '$' || line[0] == ';' {
			continue
		}
		line = strings.SplitN(line, "//", 2)[0]
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			soundMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return soundMap
}

func tryParseActor(actorScript string, lookupSprite func(soundName string) []string, lookupSound func(soundName string) []string) (DoomWeapon, error) {
	weapon, loadErr := ParseActorScript(actorScript)
	if loadErr != nil {
		return DoomWeapon{}, loadErr
	}

	neededSprites := make(map[string]bool)
	neededSounds := make(map[string]bool)

	//basePath := path.Dir(f.Name)

	for _, stateLabel := range weapon.ParsedScript {
		for _, state := range stateLabel.States {
			if state.HasSprite() {
				for _, sprite := range state.GetSpriteNames() {
					neededSprites[sprite] = true
				}
			}

			if state.HasSound() {
				soundFile := state.GetSoundFilename()
				if soundFile != "" {
					neededSounds[soundFile] = true
				}
			}
		}
		if weapon.UpSound != "" {
			neededSounds[weapon.UpSound] = true
		}
		if weapon.ReadySound != "" {
			neededSounds[weapon.ReadySound] = true
		}
		if weapon.PickupSound != "" {
			neededSounds[weapon.PickupSound] = true
		}
	}
	//spriteIndex := 0
	for _, sprite := range sortedListFromMap(neededSprites) {
		dirs := lookupSprite(sprite)
		//_, openErr := zf.Open("SPRITES/" + sprite)
		weapon.NeededSprites = append(weapon.NeededSprites, sprite)
		if len(dirs) == 0 {
			weapon.MissingSprites = append(weapon.MissingSprites, sprite)
			continue
		}

		if len(dirs) > 1 {
			println("Multiple sprites found:", sprite, dirs)
		}

		weapon.FoundSprites = append(weapon.FoundSprites, dirs[0])
	}
	for _, sound := range sortedListFromMap(neededSounds) {
		dirs := lookupSound(sound)
		weapon.NeededSounds = append(weapon.NeededSounds, sound)
		if len(dirs) == 0 {
			weapon.MissingSounds = append(weapon.MissingSounds, sound)
			continue
		}

		if len(dirs) > 1 {
			println("Multiple sounds found:", sound, dirs)
		}

		weapon.FoundSounds = append(weapon.FoundSounds, dirs[0])
	}
	return weapon, nil
}

/*
States
	{
	Ready:
		SPA2 A 1 A_WeaponReady(WRF_ALLOWRELOAD)
		Loop
	Deselect:
		SPA2 A 1 A_Lower
        TNT1 A 0 A_Lower
		Loop
	Select:
		SPA2 A 1 A_Raise
        TNT1 A 0 A_Raise
		Loop

*/
// DOCS
// https://zdoom.org/wiki/Actor_states
// https://github.com/jekyllgrim/ZScript_Basics/blob/master/12_Weapons_Overlays_PSprite.md

func ParseActorScript(fileData string) (DoomWeapon, error) {
	reader := strings.NewReader(fileData)

	//Actor SPAS12 : Weapon
	actorDefRegex := regexp.MustCompile(`(?i)actor(?-i)\s+(\w+)\s+:\s+(\w+)`)
	stateDefRegex := regexp.MustCompile(`^([\w.]+):$`)
	isReadingActor := false
	isReadingStates := false
	isReadingWeapon := false
	foundWeapon := false
	currentState := ""

	isIgnoredState := func(stateName string) bool {
		lowerState := strings.ToLower(stateName)
		if strings.Contains(lowerState, "kick") {
			return true
		}
		if strings.Contains(lowerState, "taunt") {
			return true
		}
		if strings.Contains(lowerState, "salute") {
			return true
		}
		if strings.Contains(lowerState, "grab") {
			return true
		}
		if strings.Contains(lowerState, "sprint") {
			return true
		}
		return false
	}

	isWeaponType := func(typename string) bool {
		lowerType := strings.ToLower(typename)
		if strings.Contains(lowerType, "pickup") {
			return false
		}
		if strings.Contains(lowerType, "weapon") {
			return true
		}
		return false
	}

	var commandsInCurrentState []WeaponState
	weaponFound := DoomWeapon{
		ParsedScript:   make([]WeaponScriptLabel, 0),
		OriginalScript: fileData,
	}
	lineScanner := bufio.NewScanner(reader)
	for lineScanner.Scan() {
		line := strings.TrimSpace(lineScanner.Text())
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if actorDefRegex.MatchString(line) {
			submatch := actorDefRegex.FindStringSubmatch(line)
			actorName := submatch[1]
			actorType := submatch[2]

			if isWeaponType(actorType) {
				isReadingWeapon = true
				foundWeapon = true
				weaponFound.Name = actorName
			}

			isReadingActor = true
			continue
		}
		if !isReadingActor {
			continue
		}
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "weapon.selectionorder") {
			isReadingWeapon = true
			foundWeapon = true
			continue
		}
		if !isReadingWeapon {
			continue
		}
		if strings.HasPrefix(lowerLine, "tag") {
			friendlyName := strings.Trim(line[3:], "\" ")
			weaponFound.Name = friendlyName
			continue
		}
		prefix := "weapon.readysound"
		if strings.HasPrefix(lowerLine, prefix) {
			soundName := strings.Trim(strings.TrimPrefix(line, prefix), "\" ")
			weaponFound.ReadySound = soundName
			continue
		}
		prefix = "weapon.upsound"
		if strings.HasPrefix(lowerLine, prefix) {
			soundName := strings.Trim(line[len(prefix):], "\" ")
			weaponFound.UpSound = soundName
			continue
		}
		prefix = "inventory.pickupsound"
		if strings.HasPrefix(lowerLine, prefix) {
			soundName := strings.Trim(line[len(prefix):], "\" ")
			weaponFound.PickupSound = soundName
			continue
		}
		prefix = "weapon.yadjust"
		if strings.HasPrefix(lowerLine, prefix) {
			yadjust := strings.Trim(line[len(prefix):], "\" ")
			weaponFound.YAdjust, _ = strconv.Atoi(yadjust)
			continue
		}
		if strings.Trim(lowerLine, " {") == "states" {
			isReadingStates = true
			continue
		}
		if isReadingStates && strings.ContainsRune(line, '{') {
			continue
		}
		if isReadingStates && strings.ContainsRune(line, '}') {
			isReadingStates = false
			isReadingWeapon = false
			isReadingActor = false
			if currentState != "" && !isIgnoredState(currentState) {
				weaponFound.ParsedScript = append(weaponFound.ParsedScript, WeaponScriptLabel{
					Label:  currentState,
					States: commandsInCurrentState,
				})
			}
			commandsInCurrentState = []WeaponState{}
			continue
		}
		if isReadingStates && stateDefRegex.MatchString(line) {
			if currentState != "" && !isIgnoredState(currentState) {
				weaponFound.ParsedScript = append(weaponFound.ParsedScript, WeaponScriptLabel{
					Label:  currentState,
					States: commandsInCurrentState,
				})
			}
			submatch := stateDefRegex.FindStringSubmatch(line)
			currentState = submatch[1]
			commandsInCurrentState = []WeaponState{}
			continue
		}
		if isReadingStates && currentState != "" {
			stateLine, valid := parseStateLine(line)
			if valid {
				commandsInCurrentState = append(commandsInCurrentState, stateLine)
			}

		}
	}
	if !foundWeapon {
		return weaponFound, errors.New("no weapon found")
	}
	return weaponFound, nil
}

// SPRT AV 5 A_Explode;
func parseStateLine(line string) (WeaponState, bool) {
	for _, keyword := range []string{"goto", "stop", "loop", "wait", "fail"} {
		if strings.HasPrefix(strings.ToLower(line), keyword) {
			return WeaponState{
				ControlFlow: line,
			}, true
		}
	}

	stateRegex := regexp.MustCompile(`^([\w#"]+)\s+([A-Za-z#\[\]\\"]+)\s+(-?\d+)(?:\s+(.*))?$`)
	submatch := stateRegex.FindStringSubmatch(line)
	sprite := strings.Trim(submatch[1], "\"")
	frames := strings.Trim(submatch[2], "\"")
	duration, _ := strconv.Atoi(submatch[3])

	if strings.ToLower(sprite) == "play" && strings.ToLower(frames) == "a" && duration == 0 {
		return WeaponState{}, false
	}

	function := ""
	if len(submatch) > 4 {
		function = submatch[4]
	}
	return WeaponState{
		Sprite:              sprite,
		Frame:               frames,
		DurationInTics:      duration,
		KeywordsAndFunction: function,
	}, true
}

func readSndInfo(zf *CIZipWrapper) map[string]string {
	sndInfoMap := make(map[string]string)
	sndInfoFile, sndErr := zf.Open("SNDINFO")
	if sndErr == nil {
		sndInfoMap = parseSndInfo(sndInfoFile)
		sndInfoFile.Close()
	}
	return sndInfoMap
}
