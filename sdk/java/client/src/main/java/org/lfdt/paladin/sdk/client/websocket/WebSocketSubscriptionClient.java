/*
 * Copyright contributors to Paladin, an LFDT project
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package org.lfdt.paladin.sdk.client.websocket;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.http.HttpClient;
import java.net.http.WebSocket;
import java.nio.ByteBuffer;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionStage;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.function.Consumer;
import java.util.function.Function;
import org.lfdt.paladin.sdk.client.exception.PaladinConnectionException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.client.rpc.JsonRpcRequest;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/**
 * WebSocket JSON-RPC subscriptions for receipts, blockchain events, and privacy-group messages.
 *
 * <p>Subscriptions keep a stable local handle while the server assigns a new subscription ID after
 * each reconnect. The client resubscribes automatically, with exponential backoff, and sends ping
 * frames to detect a lost connection. Each delivered batch must be acknowledged or negatively
 * acknowledged by the consumer. The server pauses that subscription until it receives one.
 *
 * <p>Callbacks run on the WebSocket listener thread. Keep them short and hand longer work to an
 * application executor. This client is thread-safe; close it when finished.
 */
public final class WebSocketSubscriptionClient implements AutoCloseable {

  private static final String PTX = "ptx";
  private static final String PGROUP = "pgroup";

  private final WebSocketClientConfig config;
  private final ObjectMapper mapper = PaladinObjectMapper.shared();
  private final HttpClient httpClient;
  private final ScheduledExecutorService scheduler =
      Executors.newSingleThreadScheduledExecutor(
          runnable -> {
            final Thread thread = new Thread(runnable, "paladin-websocket");
            thread.setDaemon(true);
            return thread;
          });
  private final Consumer<Throwable> errorHandler;
  private final Function<WebSocket.Listener, CompletableFuture<WebSocket>> connector;
  private final AtomicLong requestCounter = new AtomicLong();
  private final Map<UUID, Subscription> subscriptions = new LinkedHashMap<>();
  private final Map<String, Subscription> active = new HashMap<>();
  private final Map<String, CompletableFuture<JsonNode>> pending = new HashMap<>();
  private final CompletableFuture<Void> firstConnection = new CompletableFuture<>();

  private WebSocket socket;
  private CompletableFuture<WebSocket> sendTail = CompletableFuture.completedFuture(null);
  private ScheduledFuture<?> reconnectTask;
  private ScheduledFuture<?> heartbeatTask;
  private boolean connecting;
  private boolean closed;
  private boolean terminated;
  private long generation;
  private int reconnectAttempts;
  private long lastPongNanos;

  /**
   * Opens a WebSocket connection immediately.
   *
   * @param config the connection settings
   */
  public WebSocketSubscriptionClient(final WebSocketClientConfig config) {
    this(config, error -> {});
  }

  /**
   * Opens a WebSocket connection immediately, reporting connection and callback failures.
   *
   * @param config the connection settings
   * @param errorHandler receives asynchronous failures
   */
  public WebSocketSubscriptionClient(
      final WebSocketClientConfig config, final Consumer<Throwable> errorHandler) {
    this(config, errorHandler, null);
  }

  WebSocketSubscriptionClient(
      final WebSocketClientConfig config,
      final Consumer<Throwable> errorHandler,
      final Function<WebSocket.Listener, CompletableFuture<WebSocket>> connector) {
    this.config = Objects.requireNonNull(config, "config");
    this.errorHandler = Objects.requireNonNull(errorHandler, "errorHandler");
    httpClient = HttpClient.newBuilder().connectTimeout(config.connectTimeout()).build();
    this.connector = connector == null ? this::openSocket : connector;
    connect();
  }

  private CompletableFuture<WebSocket> openSocket(final WebSocket.Listener listener) {
    final WebSocket.Builder builder =
        httpClient.newWebSocketBuilder().connectTimeout(config.connectTimeout());
    config.headers().forEach(builder::header);
    return builder.buildAsync(config.uri(), listener);
  }

  /**
   * Completes when the first connection opens, or fails if reconnection is disabled.
   *
   * @return the first connection result
   */
  public CompletableFuture<Void> connected() {
    return firstConnection;
  }

  /**
   * Subscribes to transaction receipt batches.
   *
   * @param listenerName the server-side listener name
   * @param consumer receives batches that must be acknowledged
   * @return a stable subscription handle
   */
  public CompletableFuture<Subscription> subscribeReceipts(
      final String listenerName, final Consumer<SubscriptionEvent> consumer) {
    return subscribe(PTX, "receipts", listenerName, consumer);
  }

  /**
   * Subscribes to blockchain event batches.
   *
   * @param listenerName the server-side listener name
   * @param consumer receives batches that must be acknowledged
   * @return a stable subscription handle
   */
  public CompletableFuture<Subscription> subscribeBlockchainEvents(
      final String listenerName, final Consumer<SubscriptionEvent> consumer) {
    return subscribe(PTX, "blockchainevents", listenerName, consumer);
  }

  /**
   * Subscribes to privacy-group message batches.
   *
   * @param listenerName the server-side listener name
   * @param consumer receives batches that must be acknowledged
   * @return a stable subscription handle
   */
  public CompletableFuture<Subscription> subscribeMessages(
      final String listenerName, final Consumer<SubscriptionEvent> consumer) {
    return subscribe(PGROUP, "messages", listenerName, consumer);
  }

  private CompletableFuture<Subscription> subscribe(
      final String namespace,
      final String type,
      final String listenerName,
      final Consumer<SubscriptionEvent> consumer) {
    if (listenerName == null || listenerName.isBlank()) {
      throw new IllegalArgumentException("listenerName must not be blank");
    }
    final Subscription subscription =
        new Subscription(
            namespace, type, listenerName, Objects.requireNonNull(consumer, "consumer"));
    synchronized (this) {
      if (closed || terminated) {
        return CompletableFuture.failedFuture(new IllegalStateException("client is closed"));
      }
      subscriptions.put(subscription.localId, subscription);
      if (socket != null) {
        sendSubscribe(subscription);
      }
    }
    return subscription.ready;
  }

  private synchronized void connect() {
    if (closed || terminated || connecting || socket != null) {
      return;
    }
    connecting = true;
    final long attempt = ++generation;
    connector
        .apply(new SocketListener(attempt))
        .whenComplete(
            (opened, failure) -> {
              if (failure != null) {
                disconnected(attempt, failure);
              }
            });
  }

  private synchronized void opened(final long attempt, final WebSocket openedSocket) {
    if (closed || terminated || attempt != generation) {
      openedSocket.abort();
      return;
    }
    socket = openedSocket;
    connecting = false;
    reconnectAttempts = 0;
    sendTail = CompletableFuture.completedFuture(openedSocket);
    lastPongNanos = System.nanoTime();
    if (reconnectTask != null) {
      reconnectTask.cancel(false);
      reconnectTask = null;
    }
    heartbeatTask =
        scheduler.scheduleAtFixedRate(
            () -> heartbeat(attempt),
            config.heartbeatInterval().toMillis(),
            config.heartbeatInterval().toMillis(),
            TimeUnit.MILLISECONDS);
    for (final Subscription subscription : List.copyOf(subscriptions.values())) {
      sendSubscribe(subscription);
    }
    firstConnection.complete(null);
  }

  private synchronized void disconnected(final long attempt, final Throwable cause) {
    if (closed
        || terminated
        || attempt != generation
        || reconnectTask != null
        || (socket == null && !connecting)) {
      return;
    }
    final WebSocket oldSocket = socket;
    socket = null;
    connecting = false;
    if (oldSocket != null) {
      oldSocket.abort();
    }
    if (heartbeatTask != null) {
      heartbeatTask.cancel(false);
      heartbeatTask = null;
    }
    active.clear();
    for (final Subscription subscription : subscriptions.values()) {
      subscription.serverId = null;
    }
    final PaladinConnectionException error =
        new PaladinConnectionException("WebSocket disconnected", cause);
    for (final CompletableFuture<JsonNode> future : pending.values()) {
      future.completeExceptionally(error);
    }
    pending.clear();
    if (config.reconnectDelay() == null) {
      terminated = true;
      firstConnection.completeExceptionally(error);
      for (final Subscription subscription : subscriptions.values()) {
        subscription.ready.completeExceptionally(error);
      }
      scheduler.shutdownNow();
      report(error);
      return;
    }
    final long base = config.reconnectDelay().toMillis();
    final long max = config.maxReconnectDelay().toMillis();
    final long delay = Math.min(max, base * (1L << Math.min(reconnectAttempts++, 30)));
    reconnectTask =
        scheduler.schedule(
            () -> {
              synchronized (WebSocketSubscriptionClient.this) {
                reconnectTask = null;
              }
              connect();
            },
            delay,
            TimeUnit.MILLISECONDS);
    report(error);
  }

  private void heartbeat(final long attempt) {
    final WebSocket current;
    synchronized (this) {
      if (closed || attempt != generation || socket == null) {
        return;
      }
      current = socket;
      if (System.nanoTime() - lastPongNanos > config.heartbeatInterval().toNanos() * 3 / 2) {
        disconnected(attempt, new PaladinTimeoutException("WebSocket heartbeat timed out", null));
        return;
      }
    }
    current
        .sendPing(ByteBuffer.wrap(new byte[] {1}))
        .exceptionally(
            failure -> {
              disconnected(attempt, failure);
              return null;
            });
  }

  private void sendSubscribe(final Subscription subscription) {
    final long attempt = generation;
    request(subscription.namespace + "_subscribe", subscription.type, subscription.listenerName)
        .whenComplete(
            (result, failure) -> {
              synchronized (WebSocketSubscriptionClient.this) {
                if (closed || attempt != generation) {
                  return;
                }
                if (!subscriptions.containsKey(subscription.localId)) {
                  if (failure == null && result != null && result.isTextual()) {
                    request(subscription.namespace + "_unsubscribe", result.asText());
                  }
                  return;
                }
                if (failure != null) {
                  if (socket == null) {
                    return;
                  }
                  if (!subscription.ready.isDone()) {
                    subscriptions.remove(subscription.localId);
                    subscription.ready.completeExceptionally(failure);
                  } else {
                    report(failure);
                  }
                  return;
                }
                if (result == null || !result.isTextual() || result.asText().isBlank()) {
                  final PaladinConnectionException error =
                      new PaladinConnectionException("invalid subscription ID", null);
                  if (!subscription.ready.isDone()) {
                    subscriptions.remove(subscription.localId);
                    subscription.ready.completeExceptionally(error);
                  } else {
                    report(error);
                  }
                  return;
                }
                subscription.serverId = result.asText();
                active.put(subscription.serverId, subscription);
                subscription.ready.complete(subscription);
              }
            });
  }

  private synchronized CompletableFuture<JsonNode> request(
      final String method, final Object... params) {
    if (socket == null) {
      return CompletableFuture.failedFuture(
          new PaladinConnectionException("WebSocket is disconnected", null));
    }
    final String id = String.format(Locale.ROOT, "%09d", requestCounter.incrementAndGet());
    final CompletableFuture<JsonNode> result = new CompletableFuture<>();
    pending.put(id, result);
    scheduler.schedule(
        () -> {
          synchronized (WebSocketSubscriptionClient.this) {
            if (pending.remove(id) != null) {
              result.completeExceptionally(
                  new PaladinTimeoutException("WebSocket RPC " + method + " timed out", null));
            }
          }
        },
        config.requestTimeout().toMillis(),
        TimeUnit.MILLISECONDS);
    try {
      send(mapper.writeValueAsString(new JsonRpcRequest(id, method, List.of(params))));
    } catch (final Exception e) {
      pending.remove(id);
      result.completeExceptionally(e);
    }
    return result;
  }

  private synchronized CompletableFuture<Void> send(final String text) {
    final WebSocket current = socket;
    if (current == null) {
      return CompletableFuture.failedFuture(
          new PaladinConnectionException("WebSocket is disconnected", null));
    }
    final long attempt = generation;
    final CompletableFuture<WebSocket> next =
        sendTail
            .handle((ignored, failure) -> null)
            .thenCompose(ignored -> current.sendText(text, true));
    sendTail = next;
    next.exceptionally(
        failure -> {
          disconnected(attempt, failure);
          return null;
        });
    return next.thenApply(ignored -> null);
  }

  private void message(final long attempt, final String text) {
    final JsonNode root;
    try {
      root = mapper.readTree(text);
    } catch (final Exception e) {
      report(e);
      return;
    }
    if (root.hasNonNull("method")) {
      notification(attempt, root);
      return;
    }
    final String id = root.path("id").asText();
    final CompletableFuture<JsonNode> response;
    synchronized (this) {
      if (attempt != generation) {
        return;
      }
      response = pending.remove(id);
    }
    if (response == null) {
      return;
    }
    if (root.hasNonNull("error")) {
      final JsonNode error = root.get("error");
      response.completeExceptionally(
          new PaladinRpcException(
              error.path("code").asInt(), error.path("message").asText(), error.get("data"), 0));
    } else {
      response.complete(root.get("result"));
    }
  }

  private void notification(final long attempt, final JsonNode root) {
    final JsonNode params = root.path("params");
    final String serverId = params.path("subscription").asText();
    final Subscription subscription;
    synchronized (this) {
      if (attempt != generation) {
        return;
      }
      subscription = active.get(serverId);
    }
    if (subscription == null
        || !(subscription.namespace + "_subscription").equals(root.path("method").asText())) {
      return;
    }
    final SubscriptionEvent event =
        new SubscriptionEvent(subscription, serverId, attempt, params.get("result"));
    try {
      subscription.consumer.accept(event);
    } catch (final RuntimeException e) {
      report(e);
      event
          .nack()
          .exceptionally(
              failure -> {
                report(failure);
                return null;
              });
    }
  }

  private void report(final Throwable error) {
    try {
      errorHandler.accept(error);
    } catch (final RuntimeException ignored) {
      // An application error handler must not stop reconnection or message processing.
    }
  }

  @Override
  public synchronized void close() {
    if (closed) {
      return;
    }
    closed = true;
    generation++;
    if (reconnectTask != null) {
      reconnectTask.cancel(false);
    }
    if (heartbeatTask != null) {
      heartbeatTask.cancel(false);
    }
    final IllegalStateException error = new IllegalStateException("client is closed");
    firstConnection.completeExceptionally(error);
    for (final CompletableFuture<JsonNode> future : pending.values()) {
      future.completeExceptionally(error);
    }
    pending.clear();
    for (final Subscription subscription : subscriptions.values()) {
      subscription.ready.completeExceptionally(error);
      subscription.serverId = null;
    }
    subscriptions.clear();
    active.clear();
    if (socket != null) {
      socket.sendClose(WebSocket.NORMAL_CLOSURE, "closing");
      socket = null;
    }
    scheduler.shutdownNow();
    httpClient.close();
  }

  /** A stable local subscription handle retained across reconnections. */
  public final class Subscription {

    private final UUID localId = UUID.randomUUID();
    private final String namespace;
    private final String type;
    private final String listenerName;
    private final Consumer<SubscriptionEvent> consumer;
    private final CompletableFuture<Subscription> ready = new CompletableFuture<>();
    private volatile String serverId;

    private Subscription(
        final String namespace,
        final String type,
        final String listenerName,
        final Consumer<SubscriptionEvent> consumer) {
      this.namespace = namespace;
      this.type = type;
      this.listenerName = listenerName;
      this.consumer = consumer;
    }

    /**
     * Returns the local identifier, which stays stable after reconnection.
     *
     * @return the local identifier
     */
    public UUID localId() {
      return localId;
    }

    /**
     * Returns the current server identifier.
     *
     * @return the server identifier, or {@code null} while disconnected
     */
    public synchronized String serverId() {
      return serverId;
    }

    /**
     * Stops this subscription. While disconnected, it is removed locally and returns false.
     *
     * @return whether the server reported an active subscription
     */
    public CompletableFuture<Boolean> unsubscribe() {
      final String id;
      synchronized (WebSocketSubscriptionClient.this) {
        if (subscriptions.remove(localId) == null) {
          return CompletableFuture.completedFuture(false);
        }
        id = serverId;
        serverId = null;
        if (id != null) {
          active.remove(id);
        }
        ready.completeExceptionally(new IllegalStateException("subscription removed"));
      }
      if (id == null) {
        return CompletableFuture.completedFuture(false);
      }
      return request(namespace + "_unsubscribe", id).thenApply(JsonNode::asBoolean);
    }
  }

  /** One server batch with the server ID captured when it was delivered. */
  public final class SubscriptionEvent {

    private final Subscription subscription;
    private final String serverId;
    private final long attempt;
    private final JsonNode result;

    private SubscriptionEvent(
        final Subscription subscription,
        final String serverId,
        final long attempt,
        final JsonNode result) {
      this.subscription = subscription;
      this.serverId = serverId;
      this.attempt = attempt;
      this.result = result;
    }

    /**
     * Returns the subscription that received this batch.
     *
     * @return the stable subscription handle
     */
    public Subscription subscription() {
      return subscription;
    }

    /**
     * Returns the batch payload, including {@code batchId} and its {@code receipts}, {@code
     * events}, or {@code messages} array.
     *
     * @return the batch payload
     */
    public JsonNode result() {
      return result;
    }

    /**
     * Converts the batch payload to a Java type such as {@code TransactionReceiptBatch}, {@code
     * TransactionEventBatch}, or {@code PrivacyGroupMessageBatch}.
     *
     * @param resultType the expected batch type
     * @param <T> the batch type
     * @return the converted batch
     */
    public <T> T resultAs(final Class<T> resultType) {
      return mapper.convertValue(result, resultType);
    }

    /**
     * Acknowledges this batch on its original connection.
     *
     * @return completion of the WebSocket send
     */
    public CompletableFuture<Void> ack() {
      return respond("_ack");
    }

    /**
     * Negatively acknowledges this batch on its original connection.
     *
     * @return completion of the WebSocket send
     */
    public CompletableFuture<Void> nack() {
      return respond("_nack");
    }

    private CompletableFuture<Void> respond(final String suffix) {
      synchronized (WebSocketSubscriptionClient.this) {
        if (active.get(serverId) != subscription || attempt != generation) {
          return CompletableFuture.failedFuture(
              new PaladinConnectionException("subscription connection has changed", null));
        }
        final String id = String.format(Locale.ROOT, "%09d", requestCounter.incrementAndGet());
        try {
          return send(
              mapper.writeValueAsString(
                  new JsonRpcRequest(id, subscription.namespace + suffix, List.of(serverId))));
        } catch (final Exception e) {
          return CompletableFuture.failedFuture(e);
        }
      }
    }
  }

  private final class SocketListener implements WebSocket.Listener {

    private final long attempt;
    private final StringBuilder text = new StringBuilder();

    private SocketListener(final long attempt) {
      this.attempt = attempt;
    }

    @Override
    public void onOpen(final WebSocket webSocket) {
      opened(attempt, webSocket);
      webSocket.request(1);
    }

    @Override
    public CompletionStage<?> onText(
        final WebSocket webSocket, final CharSequence data, final boolean last) {
      text.append(data);
      if (last) {
        message(attempt, text.toString());
        text.setLength(0);
      }
      webSocket.request(1);
      return null;
    }

    @Override
    public CompletionStage<?> onPong(final WebSocket webSocket, final ByteBuffer data) {
      synchronized (WebSocketSubscriptionClient.this) {
        if (attempt == generation) {
          lastPongNanos = System.nanoTime();
        }
      }
      webSocket.request(1);
      return null;
    }

    @Override
    public CompletionStage<?> onClose(
        final WebSocket webSocket, final int statusCode, final String reason) {
      disconnected(attempt, new PaladinConnectionException("WebSocket closed: " + reason, null));
      return null;
    }

    @Override
    public void onError(final WebSocket webSocket, final Throwable error) {
      disconnected(attempt, error);
    }
  }
}
