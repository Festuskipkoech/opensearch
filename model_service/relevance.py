import math

from sentence_transformers import CrossEncoder

class Relevance:
    def __init__(self, model_path: str):
        self._model = CrossEncoder(model_path, local_files_only=True)

    def score(self, query: str, snippet: str) -> float:
        raw_logit = self._model.predict([(query, snippet)])
        return self._sigmoid(float(raw_logit[0]))

    @staticmethod
    def _sigmoid(x: float) -> float:
        return 1.0 / (1.0 + math.exp(-x))