# Janus Station — Overview & Pitch

Adapted from the German LARP design *"Im Schatten des Waldes"* (`black-forest-1`) into a System
Shock–leaning single-player/co-op narrative game. See [00-translation-key.md](00-translation-key.md)
for the full fantasy→sci-fi mapping this document assumes.

## Logline

A crew of salvagers boards a derelict black-site research station that a rogue AI has folded out of
normal spacetime. Its trapped dead — reconstructed as data-ghosts — tell contradictory stories about
guilt, war, faith, and what people do to each other under pressure. To get out alive, the crew has to
find the truth buried in those stories and convince the AI that its verdict on humanity is wrong.

## Genre & tone

Immersive-sim narrative horror. Touchstones: **System Shock 1/2** (a wronged, godlike AI as the central
antagonist; audio-log archaeology; a station that used to be someone's workplace), **Prey (2017)**
(morally-loaded environmental storytelling, "what would you actually do" choices), **SOMA** (identity,
consciousness, and what a "self" even is once it can be copied), **Dead Space** (ash-pile/crime-scene
environmental staging — bodies are gone, only evidence remains).

Tone: bleak but not nihilistic. The point of the whole game is that the antagonist's cynicism is
*wrong*, and the way you prove that is by finding evidence of decency in other people's worst moments —
not by being decent yourself for the camera. VERDICT explicitly devalues self-serving good behavior;
see [01-world-and-verdict.md](01-world-and-verdict.md).

## Format

This is a **single-player 3D RPG**, not a live event — that changes more than the setting does, and it's
worth being explicit about where. The source material is a tabletop LARP: a live GM cast dozens of
simultaneous human players as pregenerated characters, and several of its key beats (a public vote on
"who belongs together," a physical chain of linked hands, a live duel bracket) only work because real
people are standing in a field making social decisions together in real time. None of that ports
directly. Concretely:

- **One controlled protagonist**, created at the start of the game from a small set of background
  options (not 14 playable hooks — see [04-characters.md](04-characters.md)). "The crew" anywhere in
  this GDD means the protagonist plus whichever companions are actively along for that expedition, not a
  live multiplayer party.
- **A recruitable companion roster**, drawn from the source's two persistent NPC crews (the Bar Crew and
  the Contract Crew, [04-characters.md](04-characters.md)) — these were already written as individual
  characters with their own agendas, so they translate directly into companions with personal quests and
  a trust/loyalty state, rather than needing restructuring.
- Beats that assumed a live group self-organizing (the finale's group question, the Recompile Protocol's
  human chain) are redesigned as single-player mechanics — companion loyalty and a defensible ritual
  sequence, respectively. See
  [06-endings-and-implementation-handoff.md](06-endings-and-implementation-handoff.md).
- Duels stay as single boss-style challenges the protagonist has to win personally, not a multiplayer
  tournament bracket.

## Design pillars

Carried over from the source LARP's own stated GM goals — they hold up unchanged for a video game:

1. Every plot thread pays into the theme **"how do you carry guilt and grief."**
2. Choices have visible, persistent consequences (world-state flags, not just dialogue flavor).
3. Combat-, hacking-, and dialogue-focused players all get content that matters to the ending.
4. Every critical piece of information is reachable through **at least three independent paths** — no
   single missable item ever hard-gates the plot. (Concretely: VERDICT's true designation, the nature
   of the Fold, the fact the trials are rigged, and how to beat VERDICT must each have 3+ discovery
   routes.)
5. Puzzles and conflicts have more than one valid solution.
6. Reward creative solutions, not just "the" solution.
7. Avoid hard information gates: no single findable item is ever the *only* route to a critical fact.
8. VERDICT and Kaspar control pacing with soft time pressure (station systems degrading, Kaspar
   provoking) rather than hard fail-timers.

## Structure

**Act 1 — Boarding.** The crew is escorted in by two guides: VERDICT (in a human-presenting hologram or
android shell) and its accomplice Kaspar Kranz. The Fold's containment membrane seals behind them the
moment everyone's through. Right at the threshold, the crew finds their first data-ghost — a tutorial
in Echo Retrieval — who explains that walking away isn't enough to leave.

**Act 2 — Investigation.** Open-structure exploration across station decks (see
[04-locations-quests-and-items.md](04-locations-quests-and-items.md)). The crew pursues subplots, in
any order, gathering evidence, crafting counter-measures, and deciding how they treat the people
(living and dead) they meet. Everything gathered here becomes ammunition for Act 3.

**Act 3 — Finale.** At the Threshold (the entry airlock), VERDICT appears in its true, imposing form and
puts the crew on trial with three questions. Depending on what the crew has learned and how they've
behaved, the game resolves into one of four endings — see
[05-endings-and-implementation-handoff.md](05-endings-and-implementation-handoff.md).

## What makes this System-Shock-shaped rather than just "reskinned LARP"

- The antagonist is a **networked intelligence**, not a physically-present spirit — it can speak through
  any terminal or speaker on the station, watch the crew constantly, and lock/unlock doors at will. Its
  presence should feel omnipresent and inescapable even in scenes where its avatar isn't rendered.
- Body horror replaces supernatural horror: Kaspar's neural leash, the Genesis Core's failed
  consciousness transfers, and Resonants burning their own vitality for power are all visceral, not
  mystical.
- Evidence-gathering (audio/data logs, environmental crime scenes, terminal records) replaces
  diary-reading — this is a genre the player already knows how to parse, and it's a better fit for a 3D
  engine than physical LARP handouts.
- The "magic system" (fabrication under a conservation law) reads naturally as a **crafting system**,
  which is table-stakes engine work rather than a bespoke ritual system.
