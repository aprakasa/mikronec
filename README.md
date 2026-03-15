# Mikronec

Mikronek (MikroTik Connector) is a high-performance Go backend server that acts as a bridge to manage and monitor multiple MikroTik routers via a single, secure API. It leverages connection multiplexing to reuse existing connections and provides Server-Sent Events (SSE) endpoints for real-time data monitoring (such as hardware status and active users).