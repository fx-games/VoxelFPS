# Systems & Mechanics

Every system here is a direct translation of a mechanic from the source LARP (`black-forest-1/PLOT.md`,
`magic.md`, `loesungswege.md`). Function is what matters — the source treats exact recipe/word content
as a "source of truth" document separate from the main plot, and this GDD follows the same split:
mechanics here, full content list in
[04-locations-quests-and-items.md](04-locations-quests-and-items.md).

## Fabrication (replaces alchemy)

Outside the Fold, tech works normally — nanite medkits, hard-light constructs, whatever your setting's
baseline magic-equivalent is. **Inside the Fold, none of that functions.** VERDICT's field jams external
comms, cloud-dependent nanite firmware, and any consciousness-linked tech that isn't purely local. Every
survivor, augmented or not, is reduced to the station's local molecular assemblers — which are
hard-limited by a **mass-energy equivalence lock**: they cannot create matter or energy from nothing,
only reconfigure it. This is the in-fiction reason "you can't get something without sacrificing
something of equal value," ported over unchanged from the source's alchemy rule.

A successful fabrication needs, same four requirements as the source:

1. **Schematic or trained knowledge** (a blueprint file, or a character who already knows the process).
2. **Reagents as balance** — salvage components of equivalent value to the output.
3. **The Old Syntax** — a spoken/keyed activation sequence the assembler's security layer requires
   (see below).
4. **The physical synthesis act** at a fabricator terminal.

Recipe content (exact reagents, exact syntax strings, exact outputs) belongs in an implementation-side
data table, not here — treat [04-locations-quests-and-items.md](04-locations-quests-and-items.md)'s item
table as the source of truth for what each craftable item *does*, and let content/narrative design fill
in flavor text per item.

### The Old Syntax (replaces Words of Power / runes)

Pre-collapse Aurum Dynamics systems used a low-level command language for manual override — the kind of
thing normally hidden in firmware, except these systems also stamp it on physical placards near
safety-critical hardware, because in an emergency you can't count on having a working keyboard. Scavengers
found these plates engraved everywhere and, not knowing what they were, started calling them runes. They
weren't wrong to be spooked — get the syntax wrong and the assembler's failure mode is *not* graceful.

Keep the exact same letter → word → meaning table the source LARP already uses (borrowed from Ultima
Online's public "Words of Power" list) — the puzzle logic downstream (rune fragments, the `AN-` negation
prefix, combining words into command strings) depends on this exact set, and there's no reason to
invent a new one:

| Rune | Word | System meaning |
|---|---|---|
| A | An | Negate / null a running process |
| B | Bet | Small / minor scale |
| C | Corp | Terminate / kill process |
| D | Des | Down / lower |
| E | Ex | Freedom / unlock |
| F | Flam | Flame / thermal |
| G | Grav | Energy / power draw |
| H | Hur | Wind / atmospherics |
| I | In | Cause / trigger |
| J | Jux | Harm / damage flag |
| K | Kal | Summon / spawn-call |
| L | Lor | Light / illumination |
| M | Mani | Life / biotic-support |
| N | Nox | Poison / toxin |
| O | Ort | Magic → **system-level / root** |
| P | Por | Movement / actuate |
| Q | Quas | Illusion → **holo-spoof / display override** |
| R | Rel | Change / reconfigure |
| S | Sanct | Protection → **firewall / shield** |
| T | Tym | Time / delay |
| U | Uus | Raise → **uplink / elevate privilege** |
| V | Vas | Great → **amplify / broadcast-wide** |
| X | Xen | Creature → **organic/bio-matter flag** |
| W | Wis | Knowledge → **query / read** |
| Y | Ylem | Matter | 
| Z | Zu | Sleep → **suspend process** |

Command strings chain words the same way the source chains ritual formulas — e.g. `Vas Quas Sanct`
("great illusion protection" → a station-wide holo-spoofed shield) is broken by prefixing the negation
word: `An Vas Quas Sanct`. This is a direct port of the source's Stone-of-the-Wise puzzle (see Genesis
Core, below) and should stay mechanically identical.

## The Genesis Core (replaces the Stone of the Wise)

The Genesis Core looks like it bypasses the mass-energy lock — infinite output for no input — but it's
really a battery: a containment vessel charged with stolen life-energy and neural mass harvested from
murdered test subjects. Human consciousness has no measurable exchange value, so any ritual that tries
to *use* a person as balance fails, usually catastrophically. This is the exact failure mode Dr. Konrad
Erlach walks into (see Characters doc): he sacrifices his son, then more bodies, then himself, trying to
buy back his dead wife, and none of it works, because there's nothing on the other side of that scale.

Destroying a discovered Genesis Core has three valid routes, same as the source:

1. **Resolve the bound guilt.** Retrieve the Echoes of everyone the Core is built from and let their
   stories be told — without the guilt holding it together, it loses cohesion and falls apart.
2. **Anti-syntax ritual against the Core itself.** The Core's activating command is `Vas Mani Corp`
   ("great life terminate" — i.e., forced resurrection). Negate it (`An Vas Mani Corp`) while sacrificing
   an ordinary low-value power cell as balance, and the vessel discharges and collapses.
3. **Brute force**, but only with a Phase-Etchant-coated weapon or an equivalently "proven supernatural"
   effect against its housing.

## Echo Retrieval (replaces necromancy)

Trivially easy inside the Fold, because the Fold sits directly adjacent to VERDICT's cold-storage memory
lattice — the dead aren't gone, they're archived. Doesn't count as fabrication, because nothing is being
created, only played back.

Requires:

1. **A medium** — VERDICT or Kaspar, both of whom have a live neural-relay link into the lattice.
2. **The subject's name/ID** (crew manifest entry, dog tag, verbal identification).
3. **Either** their death-context, **or** a personal effect carrying neural residue (biometric dust
   bonded to an object the person handled enough — jewelry, tools, a weapon).

Some Echoes can also surface without a medium, if a player character has had a matching Anomaly Bleed
dream or vision — mirrors the source's rule that a strong-enough psychic hook can substitute for formal
summoning (e.g., a ghost that only "answers" the strongest fighter in the room).

**Data-slug detail worth keeping as a physical/visual asset:** one dead NPC (Eberwulf, see Characters
doc) had an illegal black-market memory implant in his own skull. His skull, recovered among the rubble,
still has a working socket and auto-plays a garbled looping personality recording when powered — a
direct, still-creepy port of the source's "talking skull" prop, just grounded in tech instead of magic.

## Resonants (replaces witch blood magic)

A rare post-Fold neuro-mutation lets some people burn their own bio-electric charge as an emergency,
off-grid power source — enough to jump-start dead tech or force an override the mass-energy lock would
otherwise block. Bigger effect costs more blood/vitality; it's meant to play as risky, physical,
last-resort magic, not a comfortable shortcut for every puzzle. Aggression toward a Resonant risks a
retaliatory **neural static burst** — a compulsion that drives a fighter to seek combat until they land
or take a hit — or, against a non-combatant, a **vocal jam field** that mutes them briefly.

## The Triarch Protocol (replaces Lex Trium Temporum)

A customary law for disputes with no working authority present, established generations back by
**Chief Engineer Edric Mersfeld** and generally known among people who've spent time in the Fold: form a
tribunal of the youngest, oldest, and middle-aged people present.

The source LARP resolves this by literally sorting live players into those three roles — that doesn't
apply with a single controlled protagonist. In this game it's a scripted arbitration scene: whenever a
dispute would invoke the Protocol (the Void Ranger armor claim is the clearest case, see
[05-locations-quests-and-items.md](05-locations-quests-and-items.md)), the three seats are filled by
whichever qualifying NPCs are present and reachable at that point in the story, and the protagonist
argues a side or presents evidence through normal dialogue rather than occupying a seat themselves. It
should read as world-flavor and a resolution *option* for the armor dispute, not a mechanic the player
personally performs.

## Evidence & morality tracking (drives the finale)

Every subplot resolution should write a flag: did the crew act selflessly, profit-first, or violently,
and did they surface a piece of *externally-sourced* evidence of human decency (something they didn't
personally do — a crime scene, a diary, an Echo's testimony, Kaspar's own story). The finale's Ending 1
requires roughly **three strong, independently-sourced counter-examples** — self-praise doesn't count,
per VERDICT's Audit Integrity Lock making it immune to flattery it can't verify. See
[05-endings-and-implementation-handoff.md](05-endings-and-implementation-handoff.md) for the exact
gating and a suggested flag schema.

## Combat rules vs. VERDICT and Kaspar

Neither can be hurt by ordinary weapons. **Phase Etchant** — a synthesized nanite coating, this
setting's Ghost Oil equivalent — must be applied to a weapon or ammunition; five solid Etchant-coated
hits are required to bring either of them down. If not threatened with a weapon, VERDICT does not
physically attack — this passivity is load-bearing for the Recompile Protocol ending.
