import { IncomingMessage } from "http";
import { Transform } from "stream";
import WebSocket from "ws";
import { Logger } from "./interfaces/logger";
import {
  PrivacyGroupWebSocketEvent,
  WebSocketClientOptions,
  WebSocketEvent,
  WebSocketEventCallback,
  WebSocketResult,
} from "./interfaces/websocket";

abstract class PaladinWebSocketClientBase<
  TMessageTypes extends string,
  TEvent
> {
  private logger: Logger;
  private socket: WebSocket | undefined;
  private pingTimer?: NodeJS.Timeout;
  private disconnectTimer?: NodeJS.Timeout;
  private reconnectTimer?: NodeJS.Timeout;
  private disconnectDetected = false;
  private reconnectAttempts = 0;
  private counter = 1;
  private subscriptionRequests = new Map<number, string>(); // request ID -> subscription name
  private activeSubscriptions = new Map<string, string>(); // subscription ID -> subscription name

  constructor(
    private options: WebSocketClientOptions<TMessageTypes>,
    private callback: WebSocketEventCallback<TEvent>
  ) {
    this.logger = options.logger ?? console;
    this.connect();
  }

  private connect() {
    // Clean up any old socket completely (including all listeners)
    if (this.socket) {
      this.socket.removeAllListeners();
      try {
        this.socket.close();
      } catch (e: any) {
        this.logger.warn(`Failed to close old socket: ${e.message}`);
      }
      this.socket = undefined;
    }

    // Clear any pending reconnect timer
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }

    const auth =
      this.options.username && this.options.password
        ? `${this.options.username}:${this.options.password}`
        : undefined;
    const socket = (this.socket = new WebSocket(this.options.url, {
      ...this.options.socketOptions,
      auth,
      handshakeTimeout: this.options.heartbeatInterval,
    }));

    socket
      .on("open", () => {
        this.reconnectAttempts = 0;
        if (this.disconnectDetected) {
          this.disconnectDetected = false;
          this.logger.log("Connection restored");
        } else {
          this.logger.log("Connected");
        }
        this.schedulePing();
        this.subscriptionRequests.clear();
        this.activeSubscriptions.clear();
        for (const sub of this.options.subscriptions ?? []) {
          // Automatically connect subscriptions
          const id = this.subscribe(sub.type, sub.name);
          this.subscriptionRequests.set(id, sub.name);
          this.logger.log(
            `Requested to start listening on subscription ${sub.name}`
          );
        }
        if (this.options?.afterConnect !== undefined) {
          this.options.afterConnect(this);
        }
      })
      .on("error", (err) => {
        this.logger.error("Error", err.stack);
      })
      .on("close", () => {
        // Note: this is always an unexpected close (direct calls will remove the listeners first)
        this.disconnectDetected = true;
        this.reconnect("Closed by peer");
      })
      .on("pong", () => {
        this.logger.debug && this.logger.debug(`WS received pong`);
        this.schedulePing();
      })
      .on("unexpected-response", (req, res: IncomingMessage) => {
        let responseData = "";
        res.pipe(
          new Transform({
            transform(chunk, encoding, callback) {
              responseData += chunk;
              callback();
            },
            flush: () => {
              this.reconnect(
                `Websocket connect error [${res.statusCode}]: ${responseData}`
              );
            },
          })
        );
      })
      .on("message", (data) => {
        const event: TEvent | WebSocketResult = JSON.parse(data.toString());
        if (typeof event === "object" && event !== null && "result" in event) {
          // Result of a previously sent RPC - check if it's a subscription request
          const subName = this.subscriptionRequests.get(event.id);
          if (subName) {
            const subId = event.result as string;
            this.logger.log(`Subscription ${subName} assigned ID: ${subId}`);
            this.subscriptionRequests.delete(event.id);
            this.activeSubscriptions.set(subId, subName);
          }
        } else {
          // Any other event - pass to the callback
          this.callback(this, event);
        }
      });
  }

  getSubscriptionName(subscriptionId: string) {
    return this.activeSubscriptions.get(subscriptionId);
  }

  private clearPingTimers() {
    if (this.disconnectTimer) {
      clearTimeout(this.disconnectTimer);
      delete this.disconnectTimer;
    }
    if (this.pingTimer) {
      clearTimeout(this.pingTimer);
      delete this.pingTimer;
    }
  }

  private schedulePing() {
    this.clearPingTimers();
    const heartbeatInterval = this.options.heartbeatInterval ?? 30000;
    this.disconnectTimer = setTimeout(
      () => this.reconnect("Heartbeat timeout"),
      Math.ceil(heartbeatInterval * 1.5) // 50% grace period
    );
    this.pingTimer = setTimeout(() => {
      this.logger.debug && this.logger.debug(`WS sending ping`);
      this.socket?.ping("ping", true, (err) => {
        if (err) this.reconnect(err.message);
      });
    }, heartbeatInterval);
  }

  private reconnect(msg: string) {
    if (this.reconnectTimer) {
      // Reconnect already scheduled
      return;
    }

    this.logger.error(`Websocket closed: ${msg}`);
    if (this.options.reconnectDelay === -1) {
      // Reconnection disabled - just clean up
      this.clearPingTimers();
      if (this.socket) {
        this.socket.removeAllListeners();
        try {
          this.socket.close();
        } catch (e: any) {
          this.logger.warn(`Failed to close websocket: ${e.message}`);
        }
        this.socket = undefined;
      }
      return;
    }

    // Compute reconnect delay
    const baseDelay = this.options.reconnectDelay ?? 2000;
    const maxDelay = this.options.reconnectBackoffMaxDelay;
    const delay =
      maxDelay !== undefined
        ? Math.min(baseDelay * Math.pow(2, this.reconnectAttempts), maxDelay)
        : baseDelay;

    this.reconnectAttempts += 1;
    this.reconnectTimer = setTimeout(() => this.connect(), delay);
  }

  send(json: object) {
    if (this.socket !== undefined) {
      this.socket.send(JSON.stringify(json));
    }
  }

  sendRpc(method: string, params: any[]) {
    const id = this.counter++;
    this.send({
      jsonrpc: "2.0",
      id,
      method,
      params,
    });
    return id;
  }

  async close(wait?: boolean): Promise<void> {
    this.clearPingTimers();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }

    if (!this.socket) {
      return;
    }

    const socket = this.socket;
    this.socket = undefined;

    // Remove all listeners (disables reconnect on close, etc)
    socket.removeAllListeners();

    if (wait) {
      // Add a one-time listener just for this close
      return new Promise<void>((resolve) => {
        socket.once("close", () => {
          this.logger.log("Closed");
          resolve();
        });
        try {
          socket.close();
        } catch (e: any) {
          this.logger.warn(`Failed to close websocket: ${e.message}`);
          resolve();
        }
      });
    } else {
      try {
        socket.close();
      } catch (e: any) {
        this.logger.warn(`Failed to close websocket: ${e.message}`);
      }
    }
  }

  abstract subscribe(type: TMessageTypes, name: string): number;
  abstract ack(subscription: string): void;
  abstract nack(subscription: string): void;
}

export class PaladinWebSocketClient extends PaladinWebSocketClientBase<
  "receipts" | "blockchainevents",
  WebSocketEvent
> {
  subscribe(type: "receipts" | "blockchainevents", name: string) {
    return this.sendRpc("ptx_subscribe", [type, name]);
  }

  ack(subscription: string) {
    this.sendRpc("ptx_ack", [subscription]);
  }

  nack(subscription: string) {
    this.sendRpc("ptx_nack", [subscription]);
  }
}

export class PrivacyGroupWebSocketClient extends PaladinWebSocketClientBase<
  "messages",
  PrivacyGroupWebSocketEvent
> {
  subscribe(type: "messages", name: string) {
    return this.sendRpc("pgroup_subscribe", [type, name]);
  }

  ack(subscription: string) {
    this.sendRpc("pgroup_ack", [subscription]);
  }

  nack(subscription: string) {
    this.sendRpc("pgroup_nack", [subscription]);
  }
}
