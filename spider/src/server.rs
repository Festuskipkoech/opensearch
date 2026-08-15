use std::pin::Pin;

use spider::website::Website;
use spider_transformations::transformation::content::{
    self, ReturnFormat, TransformConfig,
};
use tokio::sync::mpsc;
use tokio_stream::{wrappers::ReceiverStream, Stream};
use tonic::{Request, Response, Status};
use tracing::{error, info, warn};

use crate::proto::{
    crawler_service_server::CrawlerService, CrawlRequest, CrawlResult,
};

// Channel buffer capacity.
// One slot per URL maximum in flight- prevents the sender from getting
// ahead of a slow gRPC consumer while keeping memory bounded.
const CHANNEL_BUFFER: usize = 8;

// Per-URL fetch timeout in seconds.
// The Go engine has a 3-second overall content response target.
// Individual URL fetches get 10 seconds- the concurrency means all
// three URLs run in parallel so total wall time is bounded by the slowest.
const FETCH_TIMEOUT_SECS: u64 = 10;

pub struct CrawlerServer;

impl CrawlerServer {
    pub fn new() -> Self {
        Self
    }
}

#[tonic::async_trait]
impl CrawlerService for CrawlerServer {
    type StreamCrawlStream = Pin<Box<dyn Stream<Item = Result<CrawlResult, Status>> + Send>>;

    async fn stream_crawl(
        &self,
        request: Request<CrawlRequest>,
    ) -> Result<Response<Self::StreamCrawlStream>, Status> {
        let urls = request.into_inner().urls;

        if urls.is_empty() {
            return Err(Status::invalid_argument("urls must not be empty"));
        }

        info!(url_count = urls.len(), "stream_crawl request received");

        // mpsc channel- the producer side is moved into the spawned task,
        // the consumer side is wrapped in ReceiverStream and returned to tonic.
        // tonic drives the stream by polling the ReceiverStream; we never block
        // waiting for tonic to be ready- the channel absorbs the difference.
        let (tx, rx) = mpsc::channel::<Result<CrawlResult, Status>>(CHANNEL_BUFFER);

        tokio::spawn(async move {
            crawl_urls(urls, tx).await;
        });

        let stream = ReceiverStream::new(rx);
        Ok(Response::new(Box::pin(stream)))
    }
}

// crawl_urls fetches all URLs concurrently using one spider Website instance
// per URL (each with limit=1, depth=1 so it never follows links).
// Results are sent into the channel as each URL completes- the caller
// receives them in arrival order, not request order.
async fn crawl_urls(urls: Vec<String>, tx: mpsc::Sender<Result<CrawlResult, Status>>) {
    // Spawn one task per URL. All run concurrently on the tokio runtime.
    // We collect the handles so we can await all of them and guarantee
    // the channel sender is only dropped after every URL is done.
    let mut handles = Vec::with_capacity(urls.len());

    for url in urls {
        let tx = tx.clone();
        let handle = tokio::spawn(async move {
            fetch_one(url, tx).await;
        });
        handles.push(handle);
    }

    // Wait for every fetch to complete or be cancelled.
    // Errors from individual tasks are already sent into the channel
    // as CrawlResult with error set- we do not propagate task panics.
    for handle in handles {
        if let Err(e) = handle.await {
            error!(error = ?e, "fetch task panicked");
        }
    }
    // tx is dropped here when the last clone goes out of scope, which
    // closes the channel and signals ReceiverStream that the stream is done.
}

// fetch_one runs a single-page spider crawl for one URL.
// On success: sends a CrawlResult with markdown content and token count.
// On failure: sends a CrawlResult with error set and empty content.
// Never panics- all errors are channelled back to the caller.
async fn fetch_one(url: String, tx: mpsc::Sender<Result<CrawlResult, Status>>) {
    info!(url = %url, "fetching");

    let mut website = Website::new(&url);
    website.with_limit(1);
    website.with_depth(1);
    website.with_respect_robots_txt(false);
    website.with_request_timeout(Some(std::time::Duration::from_secs(FETCH_TIMEOUT_SECS)));

    let mut rx = website.subscribe(4);

    let tx_inner = tx.clone();
    let url_inner = url.clone();

    let consumer = tokio::spawn(async move {
        while let Ok(page) = rx.recv().await {
            let status_code = page.status_code.as_u16() as u32;

            if status_code < 200 || status_code >= 300 {
                warn!(url = %url_inner, status = status_code, "non-2xx response");
                let _ = tx_inner.send(Ok(CrawlResult {
                    url: url_inner.clone(),
                    content: String::new(),
                    status_code,
                    error: format!("HTTP {status_code}"),
                    token_count: 0,
                })).await;
                continue;
            }

            let mut transform_conf = TransformConfig::default();
            transform_conf.return_format = ReturnFormat::Markdown;
            let markdown = content::transform_content(&page, &transform_conf, &None, &None, &None);
            let token_count = (markdown.len() / 4) as u32;

            info!(url = %url_inner, status = status_code, tokens = token_count, "page fetched");

            if tx_inner.send(Ok(CrawlResult {
                url: url_inner.clone(),
                content: markdown,
                status_code,
                error: String::new(),
                token_count,
            })).await.is_err() {
                return;
            }
        }
    });

    website.crawl().await;
    website.unsubscribe();

    if let Err(e) = consumer.await {
        error!(url = %url, error = ?e, "consumer task panicked");
        send_error(&tx, &url, "internal consumer panic".to_string()).await;
    }
}

// send_error is a convenience helper for channelling fetch failures back.
// status_code 0 signals a connection-level failure (never got an HTTP response).
async fn send_error(
    tx: &mpsc::Sender<Result<CrawlResult, Status>>,
    url: &str,
    error: String,
) {
    error!(url = %url, error = %error, "fetch failed");
    let result = CrawlResult {
        url: url.to_string(),
        content: String::new(),
        status_code: 0,
        error,
        token_count: 0,
    };
    let _ = tx.send(Ok(result)).await;
}