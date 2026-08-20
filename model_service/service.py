import logging
import sys
from concurrent import futures

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

from classifier import Classifier
from relevance import Relevance

sys.path.insert(0, "../gen/models")
import models_pb2
import models_pb2_grpc

logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
log = logging.getLogger(__name__)

CLASSIFIER_PATH = "../models/classifier"
RELEVANCE_PATH  = "../models/relevance"
PORT = "[::]:50051"
MAX_WORKERS = 4


class ModelServicer(models_pb2_grpc.ModelServiceServicer):
    def __init__(self, classifier: Classifier, relevance: Relevance):
        self._classifier = classifier
        self._relevance  = relevance

    def Classify(self, request, context):
        if not request.query:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "query must not be empty")

        intent, runner_up, confidence = self._classifier.classify(request.query)
        return models_pb2.ClassifyResponse(
            intent=intent,
            runner_up=runner_up,
            confidence=confidence,
        )

    def Relevance(self, request, context):
        if not request.query or not request.snippet:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "query and snippet must not be empty")

        score = self._relevance.score(request.query, request.snippet)
        return models_pb2.RelevanceResponse(score=score)


def load_models() -> tuple[Classifier, Relevance]:
    log.info("loading classifier from %s", CLASSIFIER_PATH)
    try:
        clf = Classifier(CLASSIFIER_PATH)
    except Exception as e:
        log.error("failed to load classifier: %s", e)
        sys.exit(1)
    log.info("classifier loaded")

    log.info("loading relevance model from %s", RELEVANCE_PATH)
    try:
        rel = Relevance(RELEVANCE_PATH)
    except Exception as e:
        log.error("failed to load relevance model: %s", e)
        sys.exit(1)
    log.info("relevance model loaded")

    return clf, rel


def serve():
    # load and verify both models before binding the port
    # if either fails the process exits — no degraded startup
    clf, rel = load_models()

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=MAX_WORKERS))

    # register model service
    models_pb2_grpc.add_ModelServiceServicer_to_server(
        ModelServicer(clf, rel), server
    )

    # register standard gRPC health service
    # main.go uses this to verify the service is ready before accepting traffic
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    health_servicer.set("ModelService", health_pb2.HealthCheckResponse.SERVING)

    server.add_insecure_port(PORT)
    server.start()

    log.info("model service ready on %s", PORT)
    server.wait_for_termination()


if __name__ == "__main__":
    serve()