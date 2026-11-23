# Mock Project API

Cross-domain EV-PRES hold-out fixture (projects instead of tickets).

```powershell
go run ./cmd/mock-project
# or: BAIZE_MOCK_PROJECT_LISTEN=127.0.0.1:18081 go run ./cmd/mock-project
```

| Route | Operation |
|-------|-----------|
| `GET /projects` | `list_projects` |
| `POST /projects` | `create_project` |
| `GET /projects/{id}` | `get_project` |
| `PATCH /projects/{id}` | `update_project_status` (`active` / `archived`) |

OpenAPI: [`openapi.yaml`](openapi.yaml). The server seeds `proj-1` and `proj-2`.

Used by [`scripts/evidence-eval/cases-project.json`](../../scripts/evidence-eval/cases-project.json); the evidence-eval runner does **not** use `-CheckState` for this fixture because its state snapshot helper is ticket-shaped.
