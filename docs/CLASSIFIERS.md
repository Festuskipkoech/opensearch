# Classifiers

Two classification problems exist in the OpenSearch pipeline. Both are solved
by the Python model service, which the Go engine calls over gRPC. The Go engine
never does model inference directly — it is a gRPC client to the model service.

---

## The Model Service

A single Python service exposes two gRPC endpoints. Both models load into
memory at startup and stay loaded for the lifetime of the service.

```
proto/models/models.proto defines the contract:

  service ModelService {
    rpc Classify(ClassifyRequest) returns (ClassifyResponse)
    rpc Relevance(RelevanceRequest) returns (RelevanceResponse)
  }

  message ClassifyRequest  { string query = 1; }
  message ClassifyResponse { string intent = 1; float confidence = 2; }

  message RelevanceRequest  { string query = 1; string snippet = 2; }
  message RelevanceResponse { float score = 1; }
```

The Go engine generates a client from this proto. The Python service generates
a server. Both sides use the same contract. Changing the proto regenerates both.

---

## Classifier 1 — Intent Classification

### Model

`distilbert-base-uncased` fine-tuned on our labelled query dataset using SetFit.

```
size            66MB
inference       under 10ms on CPU
fine-tuning     opensearch-models workbench project
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
directly to the model service for validation rather than classification. The
model service confirms it is a known class and returns confidence 1.0.

### Training Data

Labelled query examples live in the opensearch-models workbench. Minimum
200 examples per class for the first model. Clean labels matter more than
volume. See opensearch-models/DATASET.md for labelling guidelines.

### Fine-Tuning

Fine-tuning happens in the opensearch-models workbench using SetFit.
SetFit is optimised for few-shot classification and trains in minutes on CPU.
When accuracy on the held-out validation set exceeds 90% across all classes
the model is ready to export.

Exported weights go into opensearch/models/classifier/.
The model service loads them at startup. No retraining happens inside opensearch.

---

## Classifier 2 — Snippet Relevance

### Model

`cross-encoder/ms-marco-MiniLM-L6-v2` — no fine-tuning needed.

```
size            22MB
inference       under 5ms per query-snippet pair on CPU
fine-tuning     none — already trained on web search relevance data
weights         downloaded from HuggingFace at container startup
```

### What It Does

Takes a query string and a snippet string together. Returns a relevance score.
High score means the snippet likely contains the answer to the query.
Low score means the snippet mentions the topic but does not answer it.

```
input:  query="what year was Redis released"
        snippet="Redis was first released in 2009 by Salvatore Sanfilippo"
output: score=0.94   <- snippet contains the answer

input:  query="explain Redis persistence tradeoffs RDB vs AOF"
        snippet="Redis supports two persistence modes"
output: score=0.31   <- snippet mentions topic but does not answer
```

### How the Crawl Decision Uses It

After SearXNG returns results the Go crawler module calls the model service
/relevance endpoint for each of the top 3 results. Scores combine with snippet
density and source authority into a final sufficiency score per intent class.

```
sufficiency = (relevance_score * 0.50)
            + (snippet_density * 0.30)
            + (source_authority * 0.20)
```

If the average sufficiency score across the top 3 results exceeds the threshold
for the detected intent class, snippets are returned immediately without invoking
Spider-rs.

```
factual     threshold 0.70
code        threshold 0.65
general     threshold 0.65
news        threshold 0.60
commercial  threshold 0.60
research    threshold 0.45
```

### Why No Fine-Tuning

The cross-encoder is trained on MS MARCO — a large-scale web passage ranking
dataset that matches our use case directly. It already scores query-snippet
relevance accurately for general web search. Fine-tuning would only be needed
if we specialised OpenSearch for a specific domain such as legal or medical.

---

## Routing — Not a Model

Routing is not a third classifier. It is a scoring function that uses the
intent class output from Classifier 1 combined with live operational signals.

Each engine has a base affinity score per intent class stored in a matrix inside
the Go router module. These start as expert-seeded values and update from real
query outcomes over time. See ARCHITECTURE.md for the full routing design.

---

## The opensearch-models Workbench

All model experimentation, fine-tuning, and evaluation happens in the separate
opensearch-models project. That project is never deployed. It produces one
output that opensearch consumes — the fine-tuned distilbert weights directory.

```
opensearch-models/    dirty work, notebooks, experiments
                      produces: models/classifier/ weights

opensearch/           production service
                      consumes: models/classifier/ weights
                      downloads: cross-encoder at container startup
```

See opensearch-models/README.md for how to run the fine-tuning pipeline.