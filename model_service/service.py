import logging
import sys
from concurrent import futures
 
import grpc
 
from classifier import Classifier
from relevance import Relevance
 
sys.path.insert(0, "../gen/models")
import models_pb2
import models_pb2_grpc
 
logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
log = logging.getLogger(__name__)
 
CLASSIFIER_PATH = "../models/classifier"
RELEVANCE_PATH  = "../models/relevance"
PORT= "[::]:50052"
MAX_WORKERS = 4

class ModelServicer(models_pb2_grpc.ModelServiceServicer):
    def __init__(self, classifier: Classifier, relevance: Relevance):
        self._classifier= classifier
        self._relevance = relevance

    def Classify(self, request, context):
        if not request.query:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "query must not be empty")

        intent, confidence =self._classifier.classify(request.query)
        return models_pb2.ClassifyResponse(intent=intent, confidence=confidence)

    def Relevance(self, request, context):
        if not request.query or not request.snippet:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "query and snippet must not be empty")

        score = self._relevance.score(request.query, request.snippet)
        return models_pb2.RelevanceResponse(score=score)

def load_models() -> tuple[Classifier, Relevance]:
    log.info("loading classifier from %s", CLASSIFIER_PATH)
    classfier = Classifier(CLASSIFIER_PATH)
    log.info("classifier loaded")

    log.info("loading relevance model from %s", RELEVANCE_PATH)
    relevance = Relevance(RELEVANCE_PATH)
    log.info("relevance model loaded")

    return classfier, relevance


def serve ():
    classifier, relevance = load_models()

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=MAX_WORKERS))
    models_pb2_grpc.add_ModelServiceServicer_to_server(
        ModelServicer(classifier, relevance), server
    )
    server.add_secure_port(PORT)
    server.start()

    log.info("model service ready on %s", PORT)
    server.wait_for_termination()

if __name__ == "__main__":
    serve()