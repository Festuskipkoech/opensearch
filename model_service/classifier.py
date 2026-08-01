from setfit import SetFitModel


VALID_CLASSES = {"news", "factual", "code", "research", "commercial", "general"}

class Classifier:
    def __init__(self, model_path: str):
        self._model = SetFitModel.from_pretrained(model_path, local_files_only=True)

    def classify(self, query: str) -> tuple[str, str, float]:
        """Returns (intent, runner_up, confidence)."""
        probabilities = self._model.predict_proba([query])[0]
        labels = self._model.labels

        scored = sorted(
            zip(labels, probabilities),
            key=lambda x: x[1],
            reverse=True,
        )

        intent     = scored[0][0]
        confidence = float(scored[0][1])
        runner_up  = scored[1][0] if len(scored) > 1 else ""

        return intent, runner_up, confidence

    def validate(self, intent: str) -> bool:
        return intent in VALID_CLASSES