# properties-v3 node panels

`web/src/components/properties-v3/` is a data-driven node property panel: a
declarative catalog (`nodeCatalog.ts`), pure engines for tabs, visibility and
diffs, and a draft state machine (`usePanelDraftState.ts`) that keeps edits in
memory until the user saves. It was built in a Codex session (requirements,
architecture and reviews were in that worktree's gitignored
`ai_docs/sessions/dev-feature-properties-v3-20260909/`) and landed on main with
one production consumer.

## What ships

| | Legacy (default) | `?panels=v3` |
|---|---|---|
| PostgreSQL node | `PostgresNodeProperties` + `PostgresConnectionInfo` | V3 `postgres` definition |
| Every other node | shipped `*NodeProperties.tsx` | unchanged — still legacy |

- `?panels=v3` turns V3 on and stores `meroku.propertyPanels=v3` in
  localStorage; `?panels=legacy` turns it off. Without the parameter, the
  stored choice applies. Default is legacy.
- The wiring is one branch in `Sidebar.tsx`. It imports only
  `properties-v3/production`, and the panel is `React.lazy`, so V3's JS and CSS
  are a separate chunk that loads only when the flag is on.
- The other 10 V3 definitions (backend, service, API Gateway, S3, CloudFront,
  Route 53, ECR, ALB, group, dynamic group) are Storybook-only
  (`Panels-v3/*`). They are not wired because:
  - their collection fields (`services[]`, `buckets[]`) have no safe YAML
    write path yet;
  - the shipped backend panel has 13 tabs (IAM, SNS, alerts, X-Ray…) that the
    V3 definition does not cover.

## Why PostgreSQL first

The shipped panels save to YAML on every keystroke
(`onChange → onConfigChange → saveConfigToBackend → PUT`). `postgres.dbname`,
`postgres.username`, `postgres.aurora`, `postgres.enabled` and
`postgres.storage_encrypted` are TF-REPLACE: the next `infra-apply` replaces
the database and loses its data. The legacy panel shows the same "saved"
indicator for these as it does for any other field.

With V3:

1. Typing changes only the in-memory draft. Nothing reaches `onConfigChange`.
2. Save on a change set that contains a TF-REPLACE field opens a confirmation
   naming the real resource (`<project>-postgres-<env> (RDS instance)` or
   `<project>-aurora-<env> (Aurora cluster)`, the legacy forms in
   `modules/postgres/naming.tf`) and the fields involved.
3. Confirm calls `onConfigChange` once, with the reviewed values. Cancel keeps
   the draft and writes nothing.
4. Non-replacement changes save without a dialog, still only on Save.

`web/src/components/properties-v3/production/__tests__/SidebarPostgres.test.tsx`
mounts the real `Sidebar` in Chromium and checks both sides: the legacy panel
writes on every keystroke, and the V3 panel follows the four steps above.

## Production binding (`production/configAdapter.ts`)

- A panel is bound to one top-level YAML block (`postgres`). A field is
  config-backed when its first property alias is a direct scalar child of that
  block. AWS-assigned and runtime fields (endpoint, port, status) have no YAML
  source, so they stay unresolved ("Resolved when created") rather than being
  invented.
- `templateDefaults` mirrors the `default` helper in `env/main.hbs`
  (`instance_class` → `db.t4g.micro`, `allocated_storage` → `20`), so the panel
  shows what will be deployed. A default is written to YAML only when the user
  changes it.
- Saves fail with an error message, rather than doing something wrong, for:
  - clearing a value — `mergeConfigUpdates` cannot delete a key, so the old
    value would silently stay;
  - collection operations;
  - fields with no YAML source;
  - the `live` and `layout` channels, which have no backend in the app.
- `lifecycleState` is always `created`. The web UI cannot tell whether the
  database exists, so it assumes it does and always gates replacement changes.
- The catalog's `affectedResources` are fixture names (`myapp_postgres_dev`).
  `bindDefinitionResources` replaces them with the bound resource. Without
  that, the confirmation would name a database that does not exist.

## Known gaps with the flag on

- No Connection tab. `PostgresConnectionInfo` is not rendered, and the
  endpoint/port rows have no data source yet.
- No pgAdmin password fetch and no monthly price labels on instance classes. The legacy
  panel shows both.
- "Saved" means handed to `App.handleConfigChange`, which writes to the backend
  fire-and-forget. A failed PUT shows in the app's global save status, not in
  the V3 save bar.

## Boundaries

`pnpm run check:property-panels-v3` (CI: Web job) enforces:

- no raw DOM in the high-level V3 files;
- no `properties-v2` imports;
- the story import allowlist;
- `Sidebar.tsx` → `properties-v3/production` is the only reference to V3 from
  outside its folder;
- `production/` never imports fixtures or stories.

## Ported from the Codex branch

V3 depended on three things from `codex/panel-settings-descriptions` that never
reached main. They were handled as follows:

- `PropertyRuntimeParameter` and `PropertyLogViewer` were copied into
  `properties-v3/primitives/RuntimePrimitives.tsx`, with their CSS.
- `PropertyOverflowText` gained an optional `className` prop in
  `components/properties`.
- The Storybook spec rail (`PropertyPanelReviewLayout`,
  `panelSpecifications.generated`) was dropped from the V3 stories.
