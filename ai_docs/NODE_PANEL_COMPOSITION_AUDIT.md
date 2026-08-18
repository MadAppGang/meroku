# Node Panel Composition Audit

Audited on 2026-08-18 against baseline commit `6182f87`.

## Boundary

A panel passes only when it:

- renders exported property Components and Layouts;
- contains no raw HTML elements or form controls;
- contains no direct `components/ui` imports;
- contains no panel-local stylesheet imports;
- contains no `className`, `style`, or `data-*` attributes.

Run the repeatable check with:

```sh
cd web
pnpm check:property-panels
```

## Shared renderer fragment

All draft node panels pass their catalog definition into the same renderer. The renderer composes the shared shell, metadata layout, save-bar component, view layout, and empty-state component:

```tsx
<PropertyPanelShell
  name={definition.name}
  meta={
    <PropertyPanelMeta
      status={definition.status}
      context={definition.context}
      tone={definition.statusTone}
    />
  }
  footer={<PropertySaveBar state={resolvedSaveState} />}
>
  <CompactPropertyView sections={compactView.sections} />
</PropertyPanelShell>
```

Fields inside `CompactPropertyView` are rendered through `PropertyGroup`, `PropertyFieldLayout`, `PropertySchemaField`, `PropertyAdvancedSettings`, `PropertyAutoscalingGroup`, `PropertyEnvironmentVariables`, `PropertyImageSource`, and `PropertyContainerProcess`.

## Panel-by-panel result

| Panel | Data-only composition fragment | Result |
|---|---|---|
| API Gateway | `draftStory(PROPERTY_NODE_CATALOG["api-gateway"])` | PASS |
| Alarms | `draftStory(PROPERTY_NODE_CATALOG.alarms)` | PASS |
| Amplify | `draftStory(PROPERTY_NODE_CATALOG.amplify)` | PASS |
| AppSync | `draftStory(PROPERTY_NODE_CATALOG.appsync)` | PASS |
| Application Load Balancer | `draftStory(PROPERTY_NODE_CATALOG.alb)` | PASS |
| Aurora | `draftStory(PROPERTY_NODE_CATALOG.aurora)` | PASS |
| Backend | `draftStory(PROPERTY_NODE_CATALOG.backend)` | PASS |
| Client App | `draftStory(PROPERTY_NODE_CATALOG["client-app"])` | PASS |
| CloudFront | `draftStory(PROPERTY_NODE_CATALOG.cloudfront)` | PASS |
| CloudWatch | `draftStory(PROPERTY_NODE_CATALOG.cloudwatch)` | PASS |
| Custom Terraform | `draftStory(PROPERTY_NODE_CATALOG["custom-terraform"])` | PASS |
| Dynamic Group | `draftStory(PROPERTY_NODE_CATALOG.dynamicGroup)` | PASS |
| ECR | `draftStory(PROPERTY_NODE_CATALOG.ecr)` | PASS |
| ECS Cluster | `draftStory(PROPERTY_NODE_CATALOG.ecs)` | PASS |
| EFS | `draftStory(PROPERTY_NODE_CATALOG.efs)` | PASS |
| EventBridge | `draftStory(PROPERTY_NODE_CATALOG.eventbridge)` | PASS |
| Event Task | `draftStory(PROPERTY_NODE_CATALOG["event-task"])` | PASS |
| GitHub | `draftStory(PROPERTY_NODE_CATALOG.github)` | PASS |
| Group | `draftStory(PROPERTY_NODE_CATALOG.group)` | PASS |
| Parameter Store | `draftStory(PROPERTY_NODE_CATALOG["secrets-manager"])` | PASS |
| PostgreSQL | `draftStory(PROPERTY_NODE_CATALOG.postgres)` | PASS |
| Route 53 | `draftStory(PROPERTY_NODE_CATALOG.route53)` | PASS |
| S3 | `draftStory(PROPERTY_NODE_CATALOG.s3)` | PASS |
| SES | `draftStory(PROPERTY_NODE_CATALOG.ses)` | PASS |
| SNS | `draftStory(PROPERTY_NODE_CATALOG.sns)` | PASS |
| SQS | `draftStory(PROPERTY_NODE_CATALOG.sqs)` | PASS |
| Scheduled Task | `draftStory(PROPERTY_NODE_CATALOG["scheduled-task"])` | PASS |
| Service | `<PropertyPanelShell><Group><PropertyFieldRow>…</PropertyFieldRow></Group></PropertyPanelShell>` | PASS |
| X-Ray | `draftStory(PROPERTY_NODE_CATALOG.xray)` | PASS |

## Service fragment

Service is the only hand-composed panel. It now uses the same library boundary:

```tsx
<PropertyPanelShell meta={<PropertyPanelMeta status="Running" />}>
  <Group title="Runtime & Scaling">
    <PropertyFieldRow variant="runtime">
      <PropertyEditableField label="Port" />
      <PropertyEditableField label="Health path" />
    </PropertyFieldRow>
    <PropertyAutoscalingGroup />
    <PropertyCapabilities />
  </Group>
  <Group
    title="Environment"
    action={<PropertyActionButton>+ Add variable</PropertyActionButton>}
  >
    <PropertyEnvironmentVariables variables={environmentVariables} />
  </Group>
  <PropertyAdvancedSettings>
    <PropertyFieldRow>…</PropertyFieldRow>
  </PropertyAdvancedSettings>
</PropertyPanelShell>
```

The fragments omit state and callbacks only for readability; the actual panel files contain no styling escape hatches.
