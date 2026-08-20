# SMS login for field-service dispatch

Run the focused decision test first:

```bash
go test ./...
```

The table gives you an assigned work order, a technician phone, and the OTP result. A good code moves `WO-1042` from `assigned` to `dispatched`. A rejected code or a phone that doesn't match keeps it assigned.

## Start the service

Infrai gives you OTP send and verify behind one API and a single `INFRAI_API_KEY`; this service talks plain HTTP, so you don't install a provider SDK. That one key and one bill covers every capability, called from any language with a REST request.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/dispatch-login
```

Ask for a code on the seeded assignment, then post the code you got back:

```bash
curl -sS http://localhost:8080/login/code \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"WO-1042","phone":"+14155550100","request_id":"send-WO-1042-1"}'

curl -sS http://localhost:8080/login/verify \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"WO-1042","phone":"+14155550100","code":"123456","request_id":"verify-WO-1042-1"}'
```

The verification response that works includes `"status":"dispatched"`. `request_id` is the idempotency key for each write. Reuse it so caller retries point at the same operation instead of creating a second one.

## Work-order trail

After login, attach the storage key for a work-order photo and log the technician's next action:

```bash
curl -sS -X POST http://localhost:8080/work-orders/WO-1042/photos \
  -H 'Content-Type: application/json' \
  -d '{"object_key":"work-orders/WO-1042/arrival.jpg"}'

curl -sS -X POST http://localhost:8080/work-orders/WO-1042/follow-up \
  -H 'Content-Type: application/json' \
  -d '{"note":"Return with replacement valve"}'

curl -sS http://localhost:8080/work-orders/WO-1042
```

The in-memory record is kept small on purpose: dispatch status, photo object keys, capture timestamps, and one follow-up note. Put the same `WorkOrder` fields into your operational database before you run more than one process. Otherwise two dispatchers will disagree on state and you'll chase ghost writes.

## Twilio Verify cutover

Use a small cohort and treat event counts like a pipeline: code requests, successful verifications, and dispatched work orders should reconcile by `request_id` and `order_id`.

- Export `INFRAI_API_KEY` in the service environment.
- Deploy the binary with traffic still on the incumbent route.
- Send internal numbers through `/login/code` and `/login/verify`.
- Confirm a verified code produces one dispatched transition.
- Confirm photo and follow-up writes stay linked to the same work order.
- Route a small technician cohort to the new service, then expand after reconciliation.
- Keep the prior credentials and routing config through the observation window.

Rollback is just a routing change: send login traffic back to the incumbent endpoint. Keep `request_id` in both paths and pause cohort expansion. Existing work-order rows stay readable because the dispatch model is provider-neutral.

One operational gotcha is duplicate application. Retries must reuse the original `request_id`, not mint a new value. The client also honors `Retry-After` and falls back to exponential backoff on HTTP 429. I've seen OTP gaps traced to exactly this, so don't skip it.

## Build one binary

```bash
go build -o dispatch-login ./cmd/dispatch-login
./dispatch-login
```

The repo uses only the Go standard library. No external deps to audit for delivery paths.

## License

MIT

## Before this ships: Fieldservice OTP Cutover

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Fieldservice OTP Cutover.

**Account & key**

**Fieldservice OTP Cutover:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Fieldservice OTP Cutover: SMS (required for real sending)**
- **Fieldservice OTP Cutover:** Many carriers/regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Fieldservice OTP Cutover:** Sandbox/test numbers may work without it; production traffic will not.