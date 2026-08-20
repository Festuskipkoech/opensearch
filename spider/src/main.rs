use mimalloc::MiMalloc;

// Replace the default allocator with mimalloc.
// Spider's README recommends this explicitly- 30-60% throughput improvement
// under concurrent crawl+parse workloads.
#[global_allocator]
static GLOBAL: MiMalloc = MiMalloc;

use std::net::SocketAddr;
use tonic::transport::Server;
use tracing::info;

mod server;

// Pull in the generated gRPC types and service descriptor.
// tonic_build compiled crawler.proto into $OUT_DIR at build time.
pub mod proto {
    tonic::include_proto!("crawler");
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialise tracing. RUST_LOG controls the level — defaults to info.
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let port: u16 = std::env::var("SPIDER_PORT")
        .unwrap_or_else(|_| "50052".to_string())
        .parse()
        .expect("SPIDER_PORT must be a valid port number");

    let addr: SocketAddr = format!("0.0.0.0:{port}").parse()?;

    let crawler_service = server::CrawlerServer::new();

    info!(port = port, "spider gRPC server starting");

    Server::builder()
        .add_service(
            proto::crawler_service_server::CrawlerServiceServer::new(crawler_service),
        )
        // gRPC health check — the Go engine's waitForGRPC startup probe
        // calls this before routing any real traffic.
        .add_service(tonic_health::server::health_reporter().1)
        .serve(addr)
        .await?;

    Ok(())
}