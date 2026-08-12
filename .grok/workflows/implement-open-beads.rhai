// Implement open Beads issues via the `bd` CLI.
//
// Walks the ready queue in priority order: discover → claim → implement →
// verify → close, then re-discovers so newly unblocked work is picked up.
// Epics with open children are never "implemented" as code — only concrete
// task/bug/feature leaves (and leaf epics with no open children).
//
// Args (all optional):
//   max_beads       — max issues to fully process this run (default 50)
//   max_rounds      — hard cap on discover/implement iterations (default 80)
//   max_priority    — only work priority <= this (0=critical..4=backlog; default 4)
//   ids             — array of specific bead IDs to implement (skips discover filter
//                     except claim/show; still processes one at a time)
//   discover_only   — true: inventory ready work and exit without implementing
//   allow_git_commit — true: allow git commit after each closed bead (default true;
//                     project policy: commit after each completed bead; no push unless asked)
//   allow_git_push  — true: allow git push (default false; requires allow_git_commit)
//   include_epics   — true: allow claiming leaf epics with zero open children
//                     (default false — skip all issue_type=epic)
//
// Budget: plan ~2 agent calls per bead (discover is shared every round) plus
// headroom. Full-DAG runs need agent_budget well above 128 (e.g. 256–1024).

let meta = #{
    name: "implement-open-beads",
    description: "Implement all ready open Beads via bd: discover, claim, implement, verify, close, loop until dry",
    when_to_use: "When you want agents to drain the bd ready queue and implement open project work end-to-end using the Beads CLI.",
    phases: [
        #{ title: "Discover", detail: "bd ready / bd show — pick next implementable leaf" },
        #{ title: "Implement", detail: "claim, code, test, close one bead" },
        #{ title: "Summarize", detail: "scratch report of closed/failed/remaining" },
    ],
};

// --- schemas ---
let ready_schema = #{
    "type": "object",
    "required": ["items", "open_count", "ready_count", "notes"],
    "properties": #{
        "items": #{
            "type": "array",
            "maxItems": 15,
            "items": #{
                "type": "object",
                "required": ["id", "title", "issue_type", "priority", "why_ready"],
                "properties": #{
                    "id": #{ "type": "string" },
                    "title": #{ "type": "string" },
                    "issue_type": #{ "type": "string" },
                    "priority": #{ "type": "integer" },
                    "why_ready": #{ "type": "string" },
                    "acceptance": #{ "type": "string" },
                },
            },
        },
        "open_count": #{ "type": "integer" },
        "ready_count": #{ "type": "integer" },
        "notes": #{ "type": "string" },
    },
};

let impl_schema = #{
    "type": "object",
    "required": ["id", "outcome", "reason"],
    "properties": #{
        "id": #{ "type": "string" },
        "outcome": #{ "type": "string" },
        "reason": #{ "type": "string" },
        "tests_ok": #{ "type": "boolean" },
        "files_touched": #{
            "type": "array",
            "maxItems": 40,
            "items": #{ "type": "string" },
        },
        "follow_up_ids": #{
            "type": "array",
            "maxItems": 10,
            "items": #{ "type": "string" },
        },
    },
};

// --- args ---
let max_beads = 50;
let max_rounds = 80;
let max_priority = 4;
let discover_only = false;
let allow_git_commit = true;
let allow_git_push = false;
let include_epics = false;
let forced_ids = [];

if args != () {
    if args.max_beads != () { max_beads = args.max_beads; }
    if args.max_rounds != () { max_rounds = args.max_rounds; }
    if args.max_priority != () { max_priority = args.max_priority; }
    if args.discover_only != () { discover_only = args.discover_only; }
    if args.allow_git_commit != () { allow_git_commit = args.allow_git_commit; }
    if args.allow_git_push != () { allow_git_push = args.allow_git_push; }
    if args.include_epics != () { include_epics = args.include_epics; }
    if args.ids != () {
        for x in args.ids {
            if type_of(x) == "string" { forced_ids.push(x); }
        }
    }
}
if allow_git_push && !allow_git_commit {
    allow_git_commit = true;
}

// Shared project rules injected into every agent prompt.
let project_rules = "";
project_rules += "PROJECT RULES (mandatory):\n";
project_rules += "1. Use the `bd` CLI for ALL task tracking. Prefer --json when parsing.\n";
project_rules += "2. NEVER use `bd edit` (opens interactive editor). Use `bd update` flags.\n";
project_rules += "3. Claim atomically: `bd update <id> --claim` before coding.\n";
project_rules += "4. Close only when acceptance criteria are actually met: ";
project_rules += "`bd close <id> --reason=\"...\"`.\n";
project_rules += "5. Create follow-ups with `bd create` when work reveals new tasks; ";
project_rules += "wire deps with `bd dep add` when needed.\n";
project_rules += "6. Spec authority: docs/02-definitive-foundry-specification-revised-fable-5.md.\n";
project_rules += "7. Run `bd prime` once at start if you need project memories / session rules.\n";
project_rules += "8. Non-interactive shell: use cp -f, mv -f, rm -f (never interactive aliases).\n";
project_rules += "9. Fix pre-existing broken tests/builds you hit — do not leave the tree red.\n";
project_rules += "10. Do NOT use TodoWrite/markdown TODOs for tracking; Beads is the source of truth.\n";

let git_rules = "";
if allow_git_commit {
    git_rules += "GIT (mandatory): After you close a bead, immediately create ONE git commit for that bead's ";
    git_rules += "changes only (do not batch multiple beads). Use HEREDOC commit messages that cite the bead id ";
    git_rules += "and title. Stage only files for this bead; leave unrelated WIP unstaged. ";
    git_rules += "Never update git config. Never force-push. Never amend unless conditions met. ";
    git_rules += "Do not commit secrets.\n";
    if allow_git_push {
        git_rules += "GIT: You MAY `git push` after commit when the remote is set up.\n";
    } else {
        git_rules += "GIT: Do NOT push. Leave commits local.\n";
    }
} else {
    git_rules += "GIT: Do NOT git commit or push. Code stays as working-tree changes. ";
    git_rules += "Report what would be committed in reason.\n";
}

// --- accumulators ---
let closed_ids = [];
let failed_ids = [];
let skipped_ids = [];
let outcomes = [];
let dry_streak = 0;
let forced_index = 0;
let round = 0;

// ========== main loop ==========
while round < max_rounds {
    if closed_ids.len() >= max_beads {
        log("Reached max_beads=" + max_beads.to_string() + "; stopping.");
        break;
    }

    // Budget headroom: need at least 2 calls (discover + implement) or 1 if forced
    let b = budget();
    if b.remaining < 2 {
        log("Agent budget nearly exhausted (remaining=" + b.remaining.to_string() + "); stopping.");
        break;
    }

    phase("Discover");
    let round_label = "r" + round.to_string();
    log("Round " + round.to_string() + " discover (closed=" + closed_ids.len().to_string()
        + " failed=" + failed_ids.len().to_string() + " remaining_budget=" + b.remaining.to_string() + ")");

    let next_id = "";
    let next_title = "";
    let next_type = "";
    let next_priority = 99;
    let next_acceptance = "";

    if forced_ids.len() > 0 {
        // Walk forced list; skip already handled.
        while forced_index < forced_ids.len() {
            let cand = forced_ids[forced_index];
            forced_index += 1;
            let already = false;
            for c in closed_ids { if c == cand { already = true; } }
            for f in failed_ids { if f == cand { already = true; } }
            for s in skipped_ids { if s == cand { already = true; } }
            if !already {
                next_id = cand;
                break;
            }
        }
        if next_id == "" {
            log("Forced id list exhausted.");
            break;
        }
        // Resolve metadata for forced id.
        let show_prompt = "";
        show_prompt += project_rules;
        show_prompt += "\nTASK: Inspect a single Beads issue and return it as the sole ready item.\n";
        show_prompt += "Run exactly:\n";
        show_prompt += "  bd show ";
        show_prompt += next_id;
        show_prompt += " --json\n";
        show_prompt += "  bd stats  (for open_count)\n";
        show_prompt += "Fill items with one entry from that issue. If the id is missing/closed, ";
        show_prompt += "return items=[] and explain in notes. Use tools; do not invent fields.\n";

        let show_r = agent(show_prompt, #{
            label: "show:" + next_id,
            capability_mode: "execute",
            output_schema: ready_schema,
        });

        let show_items = ();
        if show_r != () && show_r.success && show_r.output != () && show_r.output.items != () {
            show_items = show_r.output.items;
        }
        if show_items == () || show_items.len() == 0 {
            log("Forced id " + next_id + " not actionable; skipping.");
            skipped_ids.push(next_id);
            outcomes.push(#{ id: next_id, outcome: "skipped", reason: "missing or closed" });
            continue;
        }
        let it0 = show_items[0];
        if it0.id != () { next_id = it0.id; }
        if it0.title != () { next_title = it0.title; }
        if it0.issue_type != () { next_type = it0.issue_type; }
        if it0.priority != () { next_priority = it0.priority; }
        if it0.acceptance != () { next_acceptance = it0.acceptance; }
    } else {
        // Discover ready queue.
        let excl = "";
        let ei = 0;
        for f in failed_ids {
            if ei > 0 { excl += ","; }
            excl += f;
            ei += 1;
        }
        for s in skipped_ids {
            if ei > 0 { excl += ","; }
            excl += s;
            ei += 1;
        }
        for c in closed_ids {
            if ei > 0 { excl += ","; }
            excl += c;
            ei += 1;
        }

        let disc_prompt = "";
        disc_prompt += project_rules;
        disc_prompt += "\nTASK: Find the next Beads issue(s) ready to implement. Use shell tools.\n";
        disc_prompt += "Required commands (run them; do not invent results):\n";
        disc_prompt += "  bd ready --json\n";
        disc_prompt += "  bd stats\n";
        disc_prompt += "  For each candidate you might return: bd show <id> --json\n";
        disc_prompt += "  If needed: bd children <id>  or inspect dependencies on show output\n";
        disc_prompt += "\nSELECTION RULES:\n";
        disc_prompt += "A) Only include issues that appear in `bd ready` (no open blockers).\n";
        disc_prompt += "B) Priority filter: only priority <= ";
        disc_prompt += max_priority.to_string();
        disc_prompt += " (0 is highest urgency).\n";
        disc_prompt += "C) Prefer issue_type in {task, bug, feature, chore}.\n";
        if include_epics {
            disc_prompt += "D) Epics allowed ONLY if they have ZERO open children ";
            disc_prompt += "(leaf epic / pure rollup already done). Otherwise exclude epics.\n";
        } else {
            disc_prompt += "D) EXCLUDE all issue_type=epic. Umbrella epics are closed manually ";
            disc_prompt += "after children; do not claim them as implementation work.\n";
        }
        disc_prompt += "E) Exclude these already-handled ids (comma-separated): ";
        if excl == "" {
            disc_prompt += "(none)\n";
        } else {
            disc_prompt += excl;
            disc_prompt += "\n";
        }
        disc_prompt += "F) Sort remaining by priority ascending, then id. Return up to 10 items.\n";
        disc_prompt += "G) For the first item, copy acceptance_criteria into acceptance (truncate ok).\n";
        disc_prompt += "H) why_ready must cite that bd ready listed it and the type/priority filter passed.\n";
        disc_prompt += "I) open_count/ready_count from bd stats / ready length. notes = short queue summary.\n";
        disc_prompt += "Empty items is valid ONLY after you ran bd ready and every entry was filtered out.\n";

        let disc = agent(disc_prompt, #{
            label: "discover:" + round_label,
            capability_mode: "execute",
            output_schema: ready_schema,
        });

        if disc == () || !disc.success || disc.output == () {
            log("Discover agent failed; counting as dry streak.");
            dry_streak += 1;
            if dry_streak >= 2 {
                log("Two consecutive discover failures/empty; stopping.");
                break;
            }
            round += 1;
            continue;
        }

        let disc_items = [];
        if disc.output.items != () {
            disc_items = disc.output.items;
        }
        let open_n = 0;
        let ready_n = 0;
        let notes_s = "";
        if disc.output.open_count != () { open_n = disc.output.open_count; }
        if disc.output.ready_count != () { ready_n = disc.output.ready_count; }
        if disc.output.notes != () { notes_s = disc.output.notes; }

        log("open=" + open_n.to_string()
            + " ready_raw=" + ready_n.to_string()
            + " selected=" + disc_items.len().to_string()
            + " notes=" + notes_s);

        if disc_items.len() == 0 {
            dry_streak += 1;
            log("No implementable ready items (dry_streak=" + dry_streak.to_string() + ")");
            if dry_streak >= 2 {
                log("Queue dry for two discovers; all ready work filtered or done.");
                break;
            }
            // One more try in case of transient empty (e.g. race with in_progress).
            round += 1;
            continue;
        }
        dry_streak = 0;

        // Script-side filter (agents may still leak epics).
        let picked = ();
        for it in disc_items {
            if it == () { continue; }
            if it.id == () || it.id == "" { continue; }
            let handled = false;
            for c in closed_ids { if c == it.id { handled = true; } }
            for f in failed_ids { if f == it.id { handled = true; } }
            for s in skipped_ids { if s == it.id { handled = true; } }
            if handled { continue; }
            if it.priority != () && it.priority > max_priority { continue; }
            let t = "";
            if it.issue_type != () { t = it.issue_type; }
            if !include_epics && t == "epic" { continue; }
            picked = it;
            break;
        }

        if picked == () {
            log("All discover items filtered by script rules; treating as dry.");
            dry_streak += 1;
            if dry_streak >= 2 { break; }
            round += 1;
            continue;
        }

        if picked.id != () { next_id = picked.id; }
        if picked.title != () { next_title = picked.title; }
        if picked.issue_type != () { next_type = picked.issue_type; }
        if picked.priority != () { next_priority = picked.priority; }
        if picked.acceptance != () { next_acceptance = picked.acceptance; }
    }

    if discover_only {
        log("discover_only: would implement " + next_id + " — " + next_title);
        outcomes.push(#{
            id: next_id,
            outcome: "discovered",
            reason: next_title,
            issue_type: next_type,
            priority: next_priority,
        });
        // In discover_only mode with forced list or single pass: collect all ready once.
        if forced_ids.len() == 0 {
            // Re-run is wasteful; grab full list from a second field by re-using notes.
            // Instead mark this id skipped for selection and continue until dry.
            skipped_ids.push(next_id);
        } else {
            skipped_ids.push(next_id);
        }
        round += 1;
        continue;
    }

    // ========== Implement one bead ==========
    phase("Implement");
    log("Implement " + next_id + " P" + next_priority.to_string()
        + " [" + next_type + "] " + next_title);

    let impl_prompt = "";
    impl_prompt += project_rules;
    impl_prompt += git_rules;
    impl_prompt += "\nTASK: Fully implement ONE Beads issue end-to-end.\n";
    impl_prompt += "Issue id: ";
    impl_prompt += next_id;
    impl_prompt += "\nTitle: ";
    impl_prompt += next_title;
    impl_prompt += "\nType: ";
    impl_prompt += next_type;
    impl_prompt += "\nPriority: ";
    impl_prompt += next_priority.to_string();
    impl_prompt += "\n";
    if next_acceptance != "" {
        impl_prompt += "Acceptance (from discover; re-read via bd show as authority):\n";
        impl_prompt += next_acceptance;
        impl_prompt += "\n";
    }
    impl_prompt += "\nPROCEDURE (run these; do not skip):\n";
    impl_prompt += "1) `bd show ";
    impl_prompt += next_id;
    impl_prompt += "` — read description, acceptance, deps, notes, design.\n";
    impl_prompt += "2) `bd update ";
    impl_prompt += next_id;
    impl_prompt += " --claim` — claim atomically. If claim fails because another ";
    impl_prompt += "assignee holds it, outcome=skipped with reason.\n";
    impl_prompt += "3) Read AGENTS.md / CLAUDE.md and any linked spec sections. ";
    impl_prompt += "Use read_file/grep; do not invent repo layout.\n";
    impl_prompt += "4) Implement the issue completely against acceptance criteria and the ";
    impl_prompt += "spec authority doc. Prefer small, coherent changes.\n";
    impl_prompt += "5) Run the project's tests/builds for the touched area. Fix failures ";
    impl_prompt += "you cause or pre-existing ones you hit that block green.\n";
    impl_prompt += "6) If acceptance is NOT met, do NOT close. outcome=failed with a ";
    impl_prompt += "concrete reason and list files_touched. Leave claim in progress or ";
    impl_prompt += "add a bd note explaining blockers.\n";
    impl_prompt += "7) If acceptance IS met: `bd close ";
    impl_prompt += next_id;
    impl_prompt += " --reason=\"...\"` and set outcome=closed.\n";
    impl_prompt += "8) If you create follow-up beads, put their ids in follow_up_ids.\n";
    impl_prompt += "9) outcome must be one of: closed | failed | skipped | blocked.\n";
    impl_prompt += "10) tests_ok=true only if you actually ran tests and they passed.\n";
    impl_prompt += "Do not close umbrella epics just to clear the queue. If this issue is ";
    impl_prompt += "an epic with open children, outcome=skipped.\n";

    let impl = agent(impl_prompt, #{
        label: "impl:" + next_id,
        capability_mode: "all",
        output_schema: impl_schema,
    });

    if impl == () || !impl.success || impl.output == () {
        log("Implement agent failed for " + next_id);
        failed_ids.push(next_id);
        outcomes.push(#{ id: next_id, outcome: "failed", reason: "implement agent failure" });
        round += 1;
        continue;
    }

    let out = impl.output;
    let oc = "";
    if out.outcome != () { oc = out.outcome; }
    let reason = "";
    if out.reason != () { reason = out.reason; }
    // Canned/smoke outputs may omit fields; treat empty outcome as failed only on real runs
    // after the agent returned success with a body.
    if oc == "" { oc = "failed"; }
    log("Result " + next_id + " => " + oc + " — " + reason);

    // Normalize tracking.
    if oc == "closed" {
        closed_ids.push(next_id);
    } else if oc == "skipped" || oc == "blocked" || oc == "discovered" {
        skipped_ids.push(next_id);
    } else {
        failed_ids.push(next_id);
    }

    let row = #{
        id: next_id,
        outcome: oc,
        reason: reason,
    };
    if out.tests_ok != () {
        row.tests_ok = out.tests_ok;
    }
    if out.files_touched != () {
        row.files_touched = out.files_touched;
    }
    if out.follow_up_ids != () {
        row.follow_up_ids = out.follow_up_ids;
    }
    outcomes.push(row);

    // If we keep failing the same class of work, give the user a chance to intervene.
    if failed_ids.len() >= 3 {
        let recent_fails = 0;
        let oi = outcomes.len();
        let look = 3;
        if oi < look { look = oi; }
        let j = 0;
        while j < look {
            let r = outcomes[oi - 1 - j];
            if r != () && r.outcome == "failed" { recent_fails += 1; }
            j += 1;
        }
        if recent_fails >= 3 {
            await_user("no_progress", "Three consecutive bead implementations failed. Resume to continue, or stop the workflow.");
        }
    }

    round += 1;
}

// ========== Summarize ==========
phase("Summarize");
log("Building run summary");

let report = "";
report += "# implement-open-beads summary\n\n";
report += "- rounds: ";
report += round.to_string();
report += "\n- closed: ";
report += closed_ids.len().to_string();
report += "\n- failed: ";
report += failed_ids.len().to_string();
report += "\n- skipped: ";
report += skipped_ids.len().to_string();
report += "\n- discover_only: ";
if discover_only { report += "true"; } else { report += "false"; }
report += "\n- allow_git_commit: ";
if allow_git_commit { report += "true"; } else { report += "false"; }
report += "\n\n## Closed\n";
if closed_ids.len() == 0 {
    report += "(none)\n";
} else {
    for c in closed_ids {
        report += "- ";
        report += c;
        report += "\n";
    }
}
report += "\n## Failed\n";
if failed_ids.len() == 0 {
    report += "(none)\n";
} else {
    for f in failed_ids {
        report += "- ";
        report += f;
        report += "\n";
    }
}
report += "\n## Skipped / discovered\n";
if skipped_ids.len() == 0 {
    report += "(none)\n";
} else {
    for s in skipped_ids {
        report += "- ";
        report += s;
        report += "\n";
    }
}
report += "\n## Outcomes\n";
for o in outcomes {
    if o == () { continue; }
    let oid = "";
    let ooc = "";
    let ors = "";
    if o.id != () { oid = o.id; }
    if o.outcome != () { ooc = o.outcome; }
    if o.reason != () { ors = o.reason; }
    report += "- ";
    report += oid;
    report += " [";
    report += ooc;
    report += "]: ";
    report += ors;
    report += "\n";
}
report += "\n## Next steps\n";
report += "```bash\nbd ready\nbd stats\ngit status\n```\n";
if !allow_git_commit {
    report += "Commits were disabled; review the working tree and commit deliberately.\n";
}

let report_path = write_scratch_file("implement-open-beads-summary.md", report);
log("Wrote " + report_path);

// Final live snapshot (best-effort, single agent).
let final_notes = "";
if budget().remaining >= 1 {
    let snap_prompt = "";
    snap_prompt += "Run `bd stats` and `bd ready --json`. Return a short notes string: ";
    snap_prompt += "open/in_progress/closed counts and the top 5 ready non-epic ids. ";
    snap_prompt += "items can be empty; put the narrative in notes. open_count/ready_count required.\n";
    let snap = agent(snap_prompt, #{
        label: "final-stats",
        capability_mode: "execute",
        output_schema: ready_schema,
    });
    if snap != () && snap.success && snap.output.notes != () {
        final_notes = snap.output.notes;
    }
}

complete(#{
    summary: "closed=" + closed_ids.len().to_string()
        + " failed=" + failed_ids.len().to_string()
        + " skipped=" + skipped_ids.len().to_string()
        + " rounds=" + round.to_string(),
    report_path: report_path,
    closed: closed_ids,
    failed: failed_ids,
    skipped: skipped_ids,
    outcomes: outcomes,
    final_notes: final_notes,
    discover_only: discover_only,
});
