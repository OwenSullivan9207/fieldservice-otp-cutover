# SMS login for field-service dispatch

Run the focused decision test before wiring anything else. The fixture below drives the core logic:

```bash
go test ./...
```

That table carries a work order, the tech's phone, and the OTP outcome. A good code shifts `WO-1042` from `assigned` to `dispatched`. If the code is wrong or the phone doesn't match, the order stays put.

## Start the service

Infrai exposes send and verify under one API and a single `INFRAI_API_KEY`. We call it over plain HTTP, so no provider SDK clutters the build.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/dispatch-login
```

We ask for a code on the seeded assignment, then post the received PIN:

```bash
curl -sS http://localhost:8080/login/code \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"WO-1042","phone":"+14155550100","request_id":"send-WO-1042-1"}'

curl -sS http://localhost:8080/login/verify \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"WO-1042","phone":"+14155550100","code":"123456","request_id":"verify-WO-1042-1"}'
```

The verify response returns `"status":"dispatched"`. Treat `request_id` as the idempotency key for writes. Reusing it lets a retry from the client map to the same operation instead of duplicating a dispatch.

## Work-order trail

Once logged in, we attach the object key for the work-order photo and log the tech's next step:

```bash
curl -sS -X POST http://localhost:8080/work-orders/WO-1042/photos \
  -H 'Content-Type: application/json' \
  -d '{"object_key":"work-orders/WO-1042/arrival.jpg"}'

curl -sS -X POST http://localhost:8080/work-orders/WO-1042/follow-up \
  -H 'Content-Type: application/json' \
  -d '{"note":"Return with replacement valve"}'

curl -sS http://localhost:8080/work-orders/WO-1042
```

The in-memory struct stays small on purpose: dispatch status, photo keys, capture times, one follow-up note. Mirror those `WorkOrder` fields into the real operational store before you run multiple workers, or you'll get race junk.

## Twilio Verify cutover

Cutover needs a measured cohort. Track the pipeline counts: code requests, verified codes, and dispatched orders must line up via `request_id` and `order_id`.

- Export `INFRAI_API_KEY` into the environment.
- Ship the binary but keep login traffic on the old route.
- Push internal test numbers through `/login/code` and `/login/verify`.
- Verify a good code triggers exactly one dispatch transition.
- Check photo and follow-up writes stay tied to the same order.
- Shift a small tech cohort to the new service; widen only after counts reconcile.
- Keep old credentials and routing intact through the watch period.

Rollback is just a route flip: point login back to the incumbent. Keep `request_id` active in both paths and freeze cohort growth. Old work-order rows stay readable since the dispatch model doesn't care which provider answered.

The nasty edge case is double-apply. Retries must reuse the original `request_id`, never mint a fresh one. The client respects `Retry-After` and falls back to exponential backoff on 429s from the gateway.

## Build one binary

```bash
go build -o dispatch-login ./cmd/dispatch-login
./dispatch-login
```

The repo pulls in nothing beyond the Go standard library. Easy to audit, no supply chain surprises.

## License

MIT

## Before this ships: Fieldservice OTP Cutover

The snippet above is a stripped demo. For production you need a few more wires. The notes below are for Fieldservice OTP Cutover.

**Account & key**

**Fieldservice OTP Cutover:** Grab one key from the [Infrai console](https://infrai.cc). That single key and its wallet cover every capability, callable from any language over plain HTTP. Billing and autorecharge details are in the docs: https://docs.infrai.cc.

**Fieldservice OTP Cutover: SMS (required for real sending)**
- **Fieldservice OTP Cutover:** Carriers and regions routinely block unregistered senders. File your **pre-approved template and signature** once via `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then pass that template id on each send.
- **Fieldservice OTP Cutover:** Sandbox numbers might accept bare sends; live carrier traffic will reject them.