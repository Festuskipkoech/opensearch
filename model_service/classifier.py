from setfit import SetFitModel

VALID_CLASSES = {"news", "factual", "code", "research", "commercial", "general"}

class Classifier:
    def __init__(self, model_path: str):
        self._model = SetFitModel.from_pretrained(model_path, local_files_only=True)

    def classify(self, query: str) -> tuple[str, float]:
        probabilities = self._model.predict_proba([query])[0]
        labels = self._model.labels

        best_idx = int(probabilities.argmax())
        intent = labels[best_idx]
        confidence = float(probabilities[best_idx])

        return intent, confidence

    def validate(self, intent: str) -> bool:
        return intent in VALID_CLASSES