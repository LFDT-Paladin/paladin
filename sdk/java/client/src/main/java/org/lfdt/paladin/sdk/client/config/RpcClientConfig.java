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

package org.lfdt.paladin.sdk.client.config;

import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Connection configuration for the HTTP JSON-RPC transport, mirroring Go's {@code pldconf.HTTPClientConfig}.
 *
 * <p>Immutable; build with {@link #builder(String)}. The node URL is required; the connect and request
 * timeouts, extra HTTP headers, and {@link RetryPolicy} all default to sensible values (timeouts 30s each,
 * matching Go's {@code DefaultHTTPConfig}). Supplied headers are sent on every request — use them for
 * authentication (e.g. a bearer token) or routing metadata.
 */
public final class RpcClientConfig {

    /** The default timeout for establishing the TCP/TLS connection. */
    public static final Duration DEFAULT_CONNECT_TIMEOUT = Duration.ofSeconds(30);

    /** The default timeout for awaiting a complete response. */
    public static final Duration DEFAULT_REQUEST_TIMEOUT = Duration.ofSeconds(30);

    private final String url;
    private final Duration connectTimeout;
    private final Duration requestTimeout;
    private final Map<String, String> headers;
    private final RetryPolicy retryPolicy;

    private RpcClientConfig(Builder b) {
        this.url = b.url;
        this.connectTimeout = b.connectTimeout;
        this.requestTimeout = b.requestTimeout;
        this.headers = Map.copyOf(b.headers);
        this.retryPolicy = b.retryPolicy;
    }

    /**
     * Returns a new builder for the given node URL.
     *
     * @param url the JSON-RPC endpoint URL (e.g. {@code http://localhost:8548})
     * @return a builder pre-populated with the SDK defaults
     */
    public static Builder builder(String url) {
        return new Builder(url);
    }

    /** The JSON-RPC endpoint URL. */
    public String url() {
        return url;
    }

    /** The timeout for establishing the connection. */
    public Duration connectTimeout() {
        return connectTimeout;
    }

    /** The timeout for awaiting a complete response. */
    public Duration requestTimeout() {
        return requestTimeout;
    }

    /** The extra HTTP headers sent on every request; never null, possibly empty, unmodifiable. */
    public Map<String, String> headers() {
        return headers;
    }

    /** The retry policy governing transient-failure backoff. */
    public RetryPolicy retryPolicy() {
        return retryPolicy;
    }

    /** Builder for {@link RpcClientConfig}, pre-populated with the SDK defaults. */
    public static final class Builder {

        private final String url;
        private Duration connectTimeout = DEFAULT_CONNECT_TIMEOUT;
        private Duration requestTimeout = DEFAULT_REQUEST_TIMEOUT;
        private final Map<String, String> headers = new LinkedHashMap<>();
        private RetryPolicy retryPolicy = RetryPolicy.defaults();

        private Builder(String url) {
            this.url = Objects.requireNonNull(url, "url");
            if (url.isBlank()) {
                throw new IllegalArgumentException("url must not be blank");
            }
        }

        /**
         * Sets the timeout for establishing the connection.
         *
         * @param connectTimeout the connect timeout (must be positive)
         * @return this builder
         */
        public Builder connectTimeout(Duration connectTimeout) {
            this.connectTimeout = requirePositive(connectTimeout, "connectTimeout");
            return this;
        }

        /**
         * Sets the timeout for awaiting a complete response.
         *
         * @param requestTimeout the request timeout (must be positive)
         * @return this builder
         */
        public Builder requestTimeout(Duration requestTimeout) {
            this.requestTimeout = requirePositive(requestTimeout, "requestTimeout");
            return this;
        }

        /**
         * Adds a single HTTP header sent on every request, overwriting any previous value for that name.
         *
         * @param name the header name
         * @param value the header value
         * @return this builder
         */
        public Builder header(String name, String value) {
            this.headers.put(
                    Objects.requireNonNull(name, "header name"), Objects.requireNonNull(value, "header value"));
            return this;
        }

        /**
         * Adds all of the given HTTP headers, overwriting any previous values for matching names.
         *
         * @param headers the headers to add
         * @return this builder
         */
        public Builder headers(Map<String, String> headers) {
            headers.forEach(this::header);
            return this;
        }

        /**
         * Sets the retry policy governing transient-failure backoff.
         *
         * @param retryPolicy the retry policy
         * @return this builder
         */
        public Builder retryPolicy(RetryPolicy retryPolicy) {
            this.retryPolicy = Objects.requireNonNull(retryPolicy, "retryPolicy");
            return this;
        }

        /** Builds the immutable {@link RpcClientConfig}. */
        public RpcClientConfig build() {
            return new RpcClientConfig(this);
        }

        private static Duration requirePositive(Duration value, String name) {
            Objects.requireNonNull(value, name);
            if (value.isNegative() || value.isZero()) {
                throw new IllegalArgumentException(name + " must be positive, was " + value);
            }
            return value;
        }
    }
}
