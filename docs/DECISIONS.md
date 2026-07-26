# Architectural Decisions

---

## Modular Monolith Over Microservices

**Decision:** Ship as one Go binary with clean internal module boundaries.

**Why:** Microservices solve independent scaling and team isolation. Neither
problem exists at this stage. Internal boundaries already exist and extraction
is mechanical when real traffic data proves a bottleneck.

**When to revisit:** When profiling under real load shows a specific component
is the bottleneck and cannot be optimised within the monolith.

---

## gRPC for Spider-rs and Model Service, HTTP for SearXNG

**Decision:** Go communicates with Spider-rs and the model service over gRPC.
Go communicates with SearXNG over HTTP/JSON.

**Why gRPC for Spider-rs and model service:** We wrote both the server and the
client in each case. We define the proto contract. The compiler enforces it.
gRPC server-side streaming maps naturally to Spider-rs returning results as each
URL completes. Binary encoding is faster than JSON for high-frequency calls.

**Why HTTP for SearXNG:** SearXNG is an existing Python application with its
own HTTP API that we cannot change. We adapt to the interface it ships with.

**When to revisit:** Never for SearXNG. Only if the proto becomes a maintenance
burden for Spider-rs or the model service — unlikely given interface stability.

---

## Two Models, One Python Service

**Decision:** One Python gRPC service hosts two models — distilbert for intent
classification and cross-encoder for snippet relevance scoring.

**Why one service:** Both models are lightweight and CPU-friendly. Running them
in the same process means one container, one Dockerfile, one deployment concern,
one proto definition. Splitting into two services adds operational overhead with
no benefit at this scale.

**Why these two models:**
distilbert-base-uncased fine-tuned via SetFit gives us a classifier we fully
control, can retrain on real data, and can update without touching Go code.
cross-encoder/ms-marco-MiniLM-L6-v2 is purpose-built for query-passage relevance
scoring on web search data — our exact use case — and needs no fine-tuning.

**When to revisit:** If inference latency under load requires dedicated hardware
for one model, split them into separate services at that point.

---

## Separate Workbench Project for Model Training

**Decision:** All fine-tuning, experimentation, and evaluation happens in a
separate opensearch-models project. opensearch only consumes the exported weights.

**Why:** Fine-tuning is dirty iterative work — failed experiments, data quality
issues, hyperparameter searches. That work does not belong in a production
codebase. The boundary is clean: opensearch-models produces weights,
opensearch consumes them. A developer working on the Go engine never needs to
touch the training pipeline.

**When to revisit:** Never. This boundary should be permanent.

---

## Routing Uses an Affinity Matrix, Not a Lookup Table

**Decision:** Engine selection uses a per-intent per-engine affinity score matrix
combined with live latency and circuit breaker health signals. Scores update from
real query outcomes using an exponential moving average.

**Why not a lookup table:** A static mapping of intent to engine list never
improves and encodes assumptions that may be wrong. An affinity matrix starts
with expert knowledge and drifts toward truth as real traffic flows through.

**Why not a routing model:** Routing is a scoring problem, not a classification
problem. A simple weighted formula over a learned matrix is transparent,
debuggable, and fast. A model adds training infrastructure and opacity for no
meaningful accuracy gain over a well-tuned matrix.

**When to revisit:** If affinity score updates prove too slow to adapt to engine
quality changes. At that point a bandit algorithm replacing the EMA update is
the natural next step.

---

## Circuit Breaker State in Memory

**Decision:** Circuit breaker state per engine lives in memory inside the router.
No Redis involved.

**Why:** Circuit breaker state is operational, not persistent. Restarting the
server should reset all engines to healthy. Carrying stale OPEN state from
before a restart could block a healthy engine unnecessarily. In-memory state
means microsecond access with no network dependency.

**When to revisit:** If centralised visibility into circuit breaker state across
multiple Go binary instances is needed for operations. At that point write
circuit breaker events to a metrics system rather than storing state in Redis.

---

## Cache Write is Fire-and-Forget

**Decision:** Cache write fires in a goroutine after the response is built.
Agent does not wait for it.

**Why:** Redis writes on localhost are fast but not free. Making the agent wait
adds latency for a purely operational concern. A failed cache write means the
next identical query is a cache miss — not a correctness problem.

**When to revisit:** If cache stampede becomes a real problem under high traffic.
At that point implement singleflight to coalesce identical concurrent requests.

---

## Agent Provides Intent When Known

**Decision:** The API accepts an optional intent field. When provided, the Go
engine sends it to the model service for validation rather than classification.

**Why:** Agents already know what they want. Paying for a classification call
they have already done internally is wasteful. Trusting agent-provided intent
also allows agents to express nuance the classifier might miss.

**Risk:** A misconfigured agent providing wrong intent gets wrong routing.
The model service validates the class is known and rejects unknown values.

---

## Google Excluded from Phase 1 and 2 Engine Pool

**Decision:** Google is not included as an active engine until Phase 3.

**Why:** Google actively blocks SearXNG instances via TLS and HTTP/2
fingerprinting. After 5 consecutive requests from a fresh instance the block
triggers immediately and persists. Including Google without TLS fingerprint
rotation and a residential proxy pool means every deployment would have Google
open-circuit within seconds and never recover. A permanently open-circuited
engine adds noise and complexity with zero benefit.

Google enters the pool in Phase 3 with affinity 0.90 across all intents —
reflecting its real quality — but its circuit breaker is held in a manually
forced OPEN state until the TLS fingerprint rotation and proxy infrastructure
is confirmed working end to end.

**When to revisit:** Phase 3. Not before.

---

## DDG Included Despite Proxying Bing

**Decision:** Both DDG and Bing are included in the engine pool. The diversity
constraint prevents both from being selected in the same request.

**Why:** DDG sources results from Bing so their indexes overlap significantly.
However DDG has substantially higher blocking tolerance — its lite and html
endpoints function without JavaScript and are more resilient in a self-hosted
SearXNG context than raw Bing. The diversity constraint already solves the
duplicate result problem. Removing DDG would sacrifice blocking resilience
without gaining meaningful result diversity.

**When to revisit:** If the diversity constraint proves insufficient and
duplicate results remain a quality problem after Phase 3 affinity updates.


---

## Models Pinned Locally, Not Downloaded at Runtime

**Decision:** Both model weight directories are validated in the opensearch-models
workbench and saved to opensearch/models/. The model service loads from disk at
startup. No HuggingFace calls at runtime.

**Why:** Downloading from HuggingFace at container startup means production runs
a model that was never tested. HuggingFace can update weights silently — the
model you tested in the workbench is not guaranteed to be the model that starts
in production tomorrow. Pinning locally means what we tested is exactly what runs.
It also removes HuggingFace as a runtime dependency — the model service starts
with no external network calls required.

**When to revisit:** When model weights need to be updated. The process is always
the same — validate in the workbench, pin the new weights, copy to models/,
restart the model service container. Never update weights without workbench
validation first.

---

## Pure gRPC for Model Service, No FastAPI

**Decision:** The model service is a pure Python gRPC server. No FastAPI, no
HTTP layer, no REST endpoints.

**Why:** The only callers of the model service are internal Go modules — the
classifier client and the crawler client. Both call over gRPC. Adding an HTTP
layer adds serialization overhead, an additional dependency, and an interface
that nothing uses. gRPC gives us binary encoding, contract enforcement via proto,
and generated client code in Go. FastAPI would be the right choice if humans or
external services needed to call the model service directly — they do not.

**When to revisit:** If a debugging or monitoring use case requires human-readable
HTTP access to the model service. At that point add a reflection endpoint or a
separate lightweight debug handler — do not redesign the service boundary.
