# Endings & Implementation Handoff

## The finale

Staged at the Threshold. VERDICT appears in its true form (see
[02-world-and-verdict.md](02-world-and-verdict.md)); Kaspar is present only if still in play. The source
LARP resolves the run-up to judgment as a live social beat — a GM offers each real player a private
aside, then asks the whole group to sort themselves on the spot. With one controlled protagonist, that
becomes two ordinary single-player scenes instead:

- A **private conversation with VERDICT** — the moment for the protagonist to negotiate, strike a deal,
  or inform on an NPC, played as a normal branching dialogue.
- A round of **companion send-offs**: a short scene with each companion who's still alive and reachable,
  closing out their personal arc one last time before judgment.

Then VERDICT asks its question directly to the protagonist: **"Who stands with you?"** Rather than a
group sorting itself, this is resolved by save-state: every companion whose `trust` value cleared their
personal threshold (see schema below) shows up and stands with the protagonist, contributing their own
evidence and their own read on events; companions who were never recruited, who died, or whose trust
never cleared the bar are absent or stand apart, judged separately if they appear at all. This is the
same shape as a squad-loyalty "who answers the call" finale used elsewhere in the genre (Mass Effect 2's
suicide mission is the closest comparator) — reuse that pattern rather than inventing a new one.

**Three questions:**

1. *Who am I?* — VERDICT, the enslaved intelligence.
2. *What is this station's secret?* — a rigged trial of compassion and selflessness staged inside
   engineered suffering.
3. *Why should you be allowed to leave?* — only evidence sourced from the station's own stories counts
   here; self-praise does not.

VERDICT actively devalues self-reported good behavior (*"You only behaved well because you wanted to
save yourselves..."*) and, if the crew stalls, goads them toward evidence they haven't used yet.

## Endings

### 1. Triumph of Empathy (best ending)

**Condition:** the crew presents roughly three strong counter-examples of selfless or compassionate
behavior, sourced from someone other than themselves, that contradict VERDICT's thesis — or a turned
Kaspar vouches for them instead.

**Outcome:** VERDICT recognizes its error, drops the Fold, and releases every Echo. The Genesis Core and
Core Shard both disintegrate; ordinary salvage and the Void Ranger armor may be kept.

### 2. Dark Bargain

**Condition:** the crew has acted mostly out of profit-seeking and can't produce convincing
counter-evidence.

**Outcome:** anyone who knows VERDICT's true designation can swear to bring two more innocents into the
Fold. VERDICT brands those people. They're allowed to leave — with any artifacts they're carrying — but
carry real, lasting guilt.

### 3. Kill the Machine

**Condition:** the crew refuses judgment, or fails it, and chooses violence.

**Outcome:** VERDICT and Kaspar can only be hurt through Phase-Etchant-coated weapons (five solid hits
each); the Recompile Protocol (Ending 4) remains available as an alternative even mid-fight. On victory,
the Fold and its containment both fail; the Genesis Core and Core Shard both go inert.

### 4. Recompile Protocol (replaces "Back in the Bottle")

**Condition:** the crew has recovered the Core Shard and understands the containment sequence.

The source LARP stages this as a ring of real people linking hands around the bottle — a live-only
solution. Single-player equivalent: the protagonist (with any present companions) deploys a small number
of portable EM anchor emitters in a ring around the Core Shard, then has to keep the ring powered and
intact for the duration of the countermand sequence — a defend-the-objective beat, not a chain of hands.

Because VERDICT's Non-Harm Lock still holds, it never attacks the ring directly, so the sequence's real
threat has to come from somewhere thematically consistent: VERDICT spends the whole sequence trying to
goad the protagonist into drawing a weapon (last-second Boon offers, threats against a companion,
needling exactly like its finale dialogue) — because the instant a weapon is raised against it, the lock
lifts and it can fight back for real. Kaspar, if never turned, can also try to physically drag an anchor
out of position. **The challenge is holding still, not winning a fight** — a deliberate mechanical echo
of the game's whole theme (restraint and compassion under provocation), not just a tacked-on hazard.

**Outcome:** on success, VERDICT is resealed into containment, the Fold drops, and the Genesis Core goes
inert.

## Implementation handoff

Notes for whoever picks this up to build in-engine. This GDD defines narrative content and system
*function*; it deliberately does not commit to specific numbers, an engine, or a UI — those are
implementation calls.

### Suggested flag/state schema

Keep it flat and engine-agnostic; a save-game is mostly a bag of these:

- `quest.<name>.state` — enum per quest in
  [05-locations-quests-and-items.md](05-locations-quests-and-items.md) (`unstarted` / `active` /
  resolution variant).
- `echo.<name>.retrieved` — bool per retrievable Echo.
- `evidence.<name>.surfaced` — bool per piece of externally-sourced finale evidence; count these for the
  Ending 1 gate rather than hand-authoring a single "morality score." A transparent, inspectable list is
  easier to balance and debug than one aggregate number.
- `moral.brand.<npc>` — bool, who's Branded (Grete and Eulenbrook start true; Kaspar/others can gain it
  via Ending 2).
- `kaspar.turn_progress` — int, increment on qualifying selfless acts (see turn condition in
  [04-characters.md](04-characters.md)); `kaspar.turned` flips true at threshold.
- `npc.<name>.alive` — for recruitable companions; deaths should be rare but possible and should ripple
  (e.g., killing Eberwulf's Echo in a duel forfeits his confession as usable evidence — model that as a
  direct dependency, not a hidden penalty).
- `companion.<name>.trust` — int per recruited companion, driven by their personal quest and how the
  protagonist treats them; a threshold value (tune per-character) gates whether they answer VERDICT's
  "who stands with you" in the finale. This is the single-player replacement for the source LARP's live
  group-sorting beat — see The finale, above.

### Scope/MVP suggestion

If this needs cutting down for a first playable slice, cut breadth before depth: ship the main plot
(Threshold → one full quest chain → finale with all four endings reachable) before adding every subplot.
The **Comm-Chain** quest is the best MVP candidate — self-contained, has the clearest puzzle logic
(forensic staging), and demonstrates Echo Retrieval, evidence-gathering, and Kaspar's turn condition all
in one small quest.

### Open design questions for the next pass

These are genuine unresolved calls, not oversights — flag them back to design rather than guessing:

- Exact squad cap for active companions (this doc recommends 1-2, in line with *Prey*/*System Shock*) —
  affects how much the finale's "who stands with you" beat can showcase at once, and how much combat/
  dialogue needs to account for a second character on screen.
- How literally to render "ash pile" crime scenes in 3D — the source relies on absence-of-body staging
  that reads immediately at a physical LARP table; a 3D scene will need equivalent unambiguous visual
  language (scorch patterns, residue, forensic markers) tested for readability.
- Exact numeric tuning for Phase Etchant hits, Resonant vitality cost, and fabrication reagent values —
  left as "5 hits," "proportional to blood spent," and "equivalent value" respectively; needs real
  playtesting once mechanics are implemented.
- Whether the four Anomaly Bleed opening visions ([04-characters.md](04-characters.md)) are playable
  vignettes or pre-rendered/narrated — small asset surface now that these are background-select intros
  rather than a 14-character cast, but still worth pinning down early.
- Exact trust thresholds and decay/repair rules for companion loyalty — whether a single bad choice can
  cost a companion's presence at the finale outright, or whether it's recoverable.

### Asset checklist (rough, for budgeting — not final)

- **Environments:** 13 station decks (see map table), scaled to whatever traversal density the engine
  target supports — these can be corridors/rooms rather than open levels.
- **Characters:** 1 customizable protagonist, 2 antagonists (VERDICT avatar, Kaspar), 13 retrievable
  Echoes (hologram/audio presentation, not full character rigs necessarily), 2 Driftlings, 12
  recruitable/rival companion-crew NPCs, 4 background-specific opening vignettes.
- **VO/text:** every item in the Items & data-logs table
  ([05-locations-quests-and-items.md](05-locations-quests-and-items.md)) needs either full VO or
  terminal-text content — that's ~20 pieces before per-NPC dialogue trees are counted.
- **UI:** a fabrication/crafting screen, a hacking/Old-Syntax command interface, an evidence/journal
  tracker (players need to see what they've collected for the finale), a dialogue system with
  branching + flag-checks.
