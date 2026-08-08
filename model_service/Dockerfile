FROM python:3.11-slim-bookworm AS builder

WORKDIR /build

COPY model_service/requirements.txt .
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt


FROM python:3.11-slim-bookworm

WORKDIR /app

COPY --from=builder /install /usr/local

COPY gen/models/       ./gen/models/
COPY model_service/    ./model_service/
COPY models/classifier/ ./models/classifier/
COPY models/relevance/  ./models/relevance/

ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app

EXPOSE 50052

WORKDIR /app/model_service

CMD ["python", "service.py"]