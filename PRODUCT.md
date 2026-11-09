# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

Responsive SPA, installable as a PWA (Web Push). Native mobile clients are planned for later and are out of scope for the web design language.

## Stack

React + TypeScript, Vite, TanStack Router + Query, Tailwind, shadcn/ui, pnpm. The SPA is embedded into the Go server binary. Full details in README.md / AGENTS.md.

## Users

Primary: a technically capable self-hoster running a homelab. Usually a single person, sometimes with a few family members or friends as extra users. Alerts come from their own services: backups, CI, monitoring, smart home, cron jobs, update checkers. They often open Acksy on their phone after a push, and sometimes at a desk to clear a backlog.

## Product Purpose

Acksy is a self-hosted notification hub, in the same space as gotify and ntfy. The main job in the UI is to **clear the inbox to zero**: scan what came in, act on what matters, and mark read, archive or snooze the rest. That state is held on the server, so a notification handled on one device is handled on every device.

Success: the user trusts that zero unread means nothing needs attention, and gets back to zero in seconds.

## Positioning

- **User-first, not topic-first.** Notifications belong to a user. Topics are optional categories. gotify and ntfy are organised around apps or topics.
- **Server-side read state, synced live** (WebSocket/SSE) across all devices.
- **Deduplication and grouping**, so a flapping alert updates one entry instead of flooding the inbox.
- **Drop-in ingest compatibility** with ntfy and gotify publish APIs: existing scripts keep working.
- Scoped API tokens, OIDC login, retention policies, proper GFM markdown.

## Operating Context

- Notifications arrive from scripts and services via curl/HTTP (native API, ntfy- or gotify-compatible endpoints).
- Most sessions are short and triggered by a push on a phone. Occasional longer desk sessions for triage, plus setup of tokens and topics.
- One admin user manages users, tokens, topics and OIDC. Other users mostly just read.
- Priority runs 1–5. High-priority items must stand out without making the rest of the inbox feel alarming.

## Capabilities and Constraints

- Inbox: list, filter (topic, priority, read state, group), search, bulk mark-read (by selection, group, topic, all), archive, snooze.
- Realtime updates without reload. Changes on other devices show up immediately.
- Grouped and deduplicated items (repeat counter, last updated).
- Markdown bodies (GFM: tables, task lists, code blocks), sanitized.
- Notification actions (buttons/links) and a click URL.
- Settings: topics (with sharing), API tokens (scopes, topic limits, expiry; secret shown once), Web Push subscription, OIDC (admin).
- UI language: **English only** at launch. No i18n requirement yet.
- Light and dark themes. Keyboard-operable.

## Brand Commitments

Name: **Acksy** (from "ack": acknowledge). No logo, palette or brand assets exist yet. All of them are still to be created.

## Evidence on Hand

None: no users, testimonials, benchmarks or screenshots exist. Do not fabricate any.

## Product Principles

1. **Zero means zero.** The unread count is a promise. Never hide or soften something that needs attention.
2. **Fast triage over browsing.** Acting on a notification (read, archive, snooze, group action) takes one gesture or keystroke.
3. **Calm by default, loud when it matters.** Priority sets the volume. Routine noise stays quiet.
4. **Same state everywhere.** What the user sees on any device reflects the server immediately.
5. **Respect existing setups.** Compatibility with ntfy and gotify senders comes first. Migrating to Acksy should not require rewriting scripts.

## Accessibility & Inclusion

No formal standard set. Baseline from AGENTS.md: full keyboard operation, proper ARIA, sufficient contrast, and priority conveyed by more than color alone.
