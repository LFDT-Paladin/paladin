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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Duration;
import java.util.Map;
import org.junit.jupiter.api.Test;

class RpcClientConfigTest {

    @Test
    void retryDefaultsMirrorGo() {
        RetryPolicy p = RetryPolicy.defaults();
        assertEquals(RetryPolicy.DEFAULT_MAX_ATTEMPTS, p.maxAttempts());
        assertEquals(Duration.ofMillis(250), p.initialDelay());
        assertEquals(Duration.ofSeconds(30), p.maxDelay());
        assertEquals(2.0, p.factor());
    }

    @Test
    void delayGrowsExponentiallyAndCapsAtMaxDelay() {
        RetryPolicy p = RetryPolicy.builder()
                .initialDelay(Duration.ofMillis(100))
                .maxDelay(Duration.ofMillis(1000))
                .factor(2.0)
                .build();
        // failedAttempt=1 -> initialDelay (no multiplication), then *2 each subsequent failure.
        assertEquals(Duration.ofMillis(100), p.delayForAttempt(1));
        assertEquals(Duration.ofMillis(200), p.delayForAttempt(2));
        assertEquals(Duration.ofMillis(400), p.delayForAttempt(3));
        assertEquals(Duration.ofMillis(800), p.delayForAttempt(4));
        // 1600 would exceed the 1000 cap.
        assertEquals(Duration.ofMillis(1000), p.delayForAttempt(5));
        assertEquals(Duration.ofMillis(1000), p.delayForAttempt(6));
    }

    @Test
    void shouldRetryHonoursMaxAttempts() {
        RetryPolicy p = RetryPolicy.builder().maxAttempts(3).build();
        assertTrue(p.shouldRetry(1));
        assertTrue(p.shouldRetry(2));
        assertFalse(p.shouldRetry(3));
        assertFalse(p.shouldRetry(4));
    }

    @Test
    void singleAttemptPolicyDisablesRetries() {
        RetryPolicy p = RetryPolicy.builder().maxAttempts(1).build();
        assertFalse(p.shouldRetry(1));
    }

    @Test
    void retryPolicyRejectsInvalidValues() {
        assertThrows(IllegalArgumentException.class, () -> RetryPolicy.builder().factor(0.5));
        assertThrows(IllegalArgumentException.class,
                () -> RetryPolicy.builder().initialDelay(Duration.ofMillis(-1)));
        assertThrows(IllegalArgumentException.class,
                () -> RetryPolicy.builder().maxDelay(Duration.ofMillis(-1)));
    }

    @Test
    void configDefaultsMatchGo() {
        RpcClientConfig config = RpcClientConfig.builder("http://localhost:8548").build();
        assertEquals("http://localhost:8548", config.url());
        assertEquals(Duration.ofSeconds(30), config.connectTimeout());
        assertEquals(Duration.ofSeconds(30), config.requestTimeout());
        assertTrue(config.headers().isEmpty());
        assertEquals(RetryPolicy.defaults(), config.retryPolicy());
    }

    @Test
    void configBuilderCarriesAllOverrides() {
        RetryPolicy retry = RetryPolicy.builder().maxAttempts(2).build();
        RpcClientConfig config = RpcClientConfig.builder("http://node:1234")
                .connectTimeout(Duration.ofSeconds(5))
                .requestTimeout(Duration.ofSeconds(10))
                .header("Authorization", "Bearer token")
                .headers(Map.of("X-Trace", "abc"))
                .retryPolicy(retry)
                .build();
        assertEquals(Duration.ofSeconds(5), config.connectTimeout());
        assertEquals(Duration.ofSeconds(10), config.requestTimeout());
        assertEquals("Bearer token", config.headers().get("Authorization"));
        assertEquals("abc", config.headers().get("X-Trace"));
        assertEquals(retry, config.retryPolicy());
    }

    @Test
    void headersMapIsUnmodifiable() {
        RpcClientConfig config =
                RpcClientConfig.builder("http://node").header("a", "b").build();
        assertThrows(UnsupportedOperationException.class, () -> config.headers().put("c", "d"));
    }

    @Test
    void configRejectsInvalidValues() {
        assertThrows(IllegalArgumentException.class, () -> RpcClientConfig.builder(""));
        assertThrows(NullPointerException.class, () -> RpcClientConfig.builder(null));
        assertThrows(IllegalArgumentException.class,
                () -> RpcClientConfig.builder("http://node").requestTimeout(Duration.ZERO));
        assertThrows(IllegalArgumentException.class,
                () -> RpcClientConfig.builder("http://node").connectTimeout(Duration.ofSeconds(-1)));
    }
}
