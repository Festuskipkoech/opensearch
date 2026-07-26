# Classifiers

Two classification problems exist in the OpenSearch pipeline. Both are solved
by the Python model service, which the Go engine calls over gRPC. The Go engine
never does model inference directly — it is a gRPC client to the model service.

---

## The Model Service

A pure Python gRPC server. No FastAPI, no HTTP layer. Two models load from disk
at startup and stay loaded for the service lifetime. No HuggingFace calls at
runtime. No external network calls after startup.

### Proto Contract

```proto
syntax = "proto3";

package models;

option go_package = "opensearch/gen/models";

service ModelService {
  rpc Classify  (ClassifyRequest)  returns (ClassifyResponse);
  rpc Relevance (RelevanceRequest) returns (RelevanceResponse);
}

message ClassifyRequest  { string query   = 1; }
message ClassifyResponse { string intent  = 1; float confidence = 2; }

message RelevanceRequest  { string query   = 1; string snippet = 2; }
message RelevanceResponse { float  score   = 1; }
```

The Go engine generates a client from this proto. The Python service generates
a server. Both sides use the same contract.

### Service Structure

```
model_service/
├── service.py          gRPC server entry point
├── classifier.py       distilbert inference wrapper
├── relevance.py        cross-encoder inference wrapper — applies sigmoid internally
├── Dockerfile
└── requirements.txt
```

`service.py` loads both models before binding the gRPC port. If either model
fails to load the service exits immediately — it never starts in a degraded
state. The Go engine health check detects the failure and the server refuses
traffic until the model service is healthy.

### Startup Sequence

```
service.py starts
  |
  load distilbert from models/classifier/
  load cross-encoder from models/relevance/
  both loaded successfully
  |
  bind gRPC server to port
  ready to serve
```

### Requirements

```
grpcio
grpcio-tools
sentence-transformers
torch
```

---

## Classifier 1 — Intent Classification

### Model

`distilbert-base-uncased` fine-tuned on our labelled query dataset using SetFit.

```
size            ~253MB (SetFit packages sentence transformer wrapper)
inference       under 10ms on CPU
fine-tuning     opensearch-models workbench
weights         opensearch/models/classifier/
classes         news, factual, code, research, commercial, general
```

### What It Does

Takes a raw query string. Returns the intent class and a confidence score
between 0.0 and 1.0.

```
input:  "how to handle goroutine panic in go"
output: intent="code", confidence=0.94
```

### Confidence Handling

If confidence is below the configured threshold (default 0.65) the Go
orchestrator detects uncertainty and hedges — it queries engines suited to
the top two candidate intents and merges results. The intent in the response
is marked uncertain.

If the agent provides an intent in the request body, the Go engine sends it
to the model service for validation rather than classification. The model
service confirms it is a known class and returns confidence 1.0.

### Training

Fine-tuning happens in the opensearch-models workbench using SetFit.
Minimum 200 labelled examples per class. Test set accuracy above 90% and
per-class F1 above 0.85 before the model is considered ready to export.

Exported weights go into opensearch/models/classifier/.
No retraining inside opensearch. Retrain in the workbench, export, restart
the model service container.

---

## Classifier 2 — Snippet Relevance

### Model

`cross-encoder/ms-marco-MiniLM-L6-v2` — no fine-tuning needed.

```
size            91MB
inference       under 5ms per query-snippet pair on CPU
fine-tuning     none — trained on MS MARCO web passage ranking data
weights         opensearch/models/relevance/ (pinned, validated in workbench)
```

### What It Does

Takes a query string and a snippet string together. Returns a relevance score
normalised to 0.0 to 1.0.

The model internally produces a raw logit. The model service applies sigmoid
normalisation before returning the score over gRPC. The Go crawler module
always receives a 0-1 value and applies the sufficiency formula directly —
it never handles raw logits.

```
input:  query="what year was Redis released"
        snippet="Redis was first released in 2009 by Salvatore Sanfilippo"
output: score=0.94

input:  query="explain Redis persistence tradeoffs RDB vs AOF"
        snippet="Redis supports two persistence modes"
output: score=0.31
```

### Sigmoid Normalisation

Applied inside `relevance.py` before the score is sent over gRPC:

```python
normalised = 1 / (1 + exp(-raw_logit))
```

Maps the unbounded logit range to 0-1. The Go crawler module never sees
a raw logit. The sufficiency formula and thresholds operate on the 0-1 range.

### How the Crawl Decision Uses It

After SearXNG returns results the Go crawler module calls rpc Relevance for
each of the top 3 results. Scores combine with snippet density and source
authority into a sufficiency score.

```
sufficiency = (relevance_score  * 0.50)
            + (snippet_density  * 0.30)
            + (source_authority * 0.20)
```

If average sufficiency across the top 3 results exceeds the threshold for the
detected intent class, snippets are returned immediately without Spider-rs.

```
factual     0.70
code        0.65
general     0.65
news        0.60
commercial  0.60
research    0.45
```

### Why No Fine-Tuning

MS MARCO is a large-scale web passage ranking dataset matching our use case
directly. The model scores query-snippet relevance accurately for general web
search without any domain-specific fine-tuning. Fine-tuning is only needed
if OpenSearch is specialised for a specific domain such as legal or medical.

---

## Routing — Not a Model

Routing is not a third classifier. It is a scoring function using the intent
class from Classifier 1 combined with live operational signals. See
ARCHITECTURE.md for the full affinity matrix design.

---

## The opensearch-models Workbench

All experimentation, fine-tuning, and validation happens in the separate
opensearch-models project. It produces two outputs that opensearch consumes.

```
opensearch-models/
  notebook 02/03    fine-tunes distilbert, exports to models/classifier/
  notebook 04       validates cross-encoder, pins weights to models/relevance/

opensearch/
  models/classifier/    loaded from disk at model service startup
  models/relevance/     loaded from disk at model service startup
```

Neither model downloads anything at runtime. What the workbench tested and
pinned is exactly what production runs.
