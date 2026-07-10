import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { handleRpcMethod } from './handlers/dispatch.js';
import type { JsonRpcRequest, JsonRpcResponse } from './types.js';

const port = Number(process.env.MOCK_RPC_PORT ?? 31999);

const readBody = (req: IncomingMessage): Promise<string> =>
  new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => resolve(Buffer.concat(chunks).toString()));
    req.on('error', reject);
  });

const sendJson = (res: ServerResponse, status: number, body: unknown) => {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(body));
};

const server = createServer(async (req, res) => {
  if (req.method === 'GET' && req.url === '/health') {
    sendJson(res, 200, { status: 'ok' });
    return;
  }

  if (req.method !== 'POST') {
    sendJson(res, 405, { error: 'Method not allowed' });
    return;
  }

  try {
    const body = await readBody(req);
    const request = JSON.parse(body) as JsonRpcRequest;

    const result = await handleRpcMethod(request.method, request.params ?? []);

    const response: JsonRpcResponse = {
      jsonrpc: '2.0',
      id: request.id,
      result,
    };

    sendJson(res, 200, response);
  } catch (err) {
    console.error('[mock-rpc] error handling request:', err);
    const response: JsonRpcResponse = {
      jsonrpc: '2.0',
      id: null,
      error: {
        code: -32603,
        message: err instanceof Error ? err.message : 'Internal error',
      },
    };
    sendJson(res, 500, response);
  }
});

server.listen(port, () => {
  console.log(`[mock-rpc] listening on http://localhost:${port}`);
});
