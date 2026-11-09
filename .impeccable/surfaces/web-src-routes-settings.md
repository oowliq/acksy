---
version: 1
slug: "web-src-routes-settings"
primary_target: "web/src/routes/_app/settings/route.tsx"
related_targets: ["web/src/components/settings/nav.tsx","web/src/components/form.tsx","web/src/components/settings/token-dialog.tsx","web/src/components/settings/topic-page.tsx"]
---

# Settings (account, push, API tokens, topics, webhooks, users, sign-in)

Scope: the `/settings` area of the web app. Mode: **Operate**. Inherits the inbox direction contract (`web-src-routes-index-tsx.md`): Catppuccin Mocha/Latte, mantle sidebar, hairline structure, lavender for selection and the primary action only.

Audience and job: the same homelab self-hoster, at a desk, setting up senders (tokens), organising and sharing topics, and occasionally changing their profile. Sessions are rare and purposeful; clarity beats density.

## Layout

- Desktop: the inbox sidebar is replaced by settings navigation ("← Inbox", Settings group, Admin group for admins only). Content column max 720px.
- Phone: `/settings` is the section list as its own screen; inside a section the header shows "← Settings" and the section title. Dialogs become bottom sheets.

## Rules

- Forms are built from `components/form.tsx`: Field (label, hint, error wired with aria-describedby), Input/Textarea (sunken, 40px), Checkbox, RadioGroup, Dialog, ConfirmButton, SettingsSection.
- Each section saves with its own Save button, enabled only when something changed.
- Destructive steps use the neutral inverse `confirm` button, never red: red and peach stay reserved for priority 4–5. Deleting a topic requires typing its name; revoking a token or removing a member is a two-step ConfirmButton.
- Webhook URLs are shown as their origin only; changing one means typing a new URL.
- Secrets are shown once, in monospace, with a Copy button and ready-to-run examples. The secret dialog can't be dismissed silently.
- Members of a shared topic see the same page read-only, with Leave instead of Delete.

## Unresolved

- None for M7. Sign-in (OIDC) is one long form with a sticky Test/Save bar; the client secret behaves like a webhook URL (never shown, replaced on demand).
