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

package org.lfdt.paladin.sdk.client.exception;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.JsonNodeFactory;
import org.junit.jupiter.api.Test;

class PaladinExceptionTest {

    @Test
    void wholeHierarchyIsUncheckedAndRootedAtPaladinException() {
        // Every SDK exception must be unchecked (extends RuntimeException) and catchable as PaladinException.
        for (PaladinException ex : new PaladinException[] {
                new PaladinException("base"),
                new PaladinRpcException(-32603, "rpc", null, 200),
                new PaladinTimeoutException("timeout"),
                new PaladinConnectionException("connection")
        }) {
            assertInstanceOf(RuntimeException.class, ex);
        }
    }

    @Test
    void carriesMessageAndCause() {
        Throwable cause = new IllegalStateException("boom");
        PaladinConnectionException ex = new PaladinConnectionException("refused", cause);
        assertEquals("refused", ex.getMessage());
        assertSame(cause, ex.getCause());
    }

    @Test
    void rpcExceptionExposesCodeStatusAndData() {
        JsonNode data = JsonNodeFactory.instance.numberNode(42);
        PaladinRpcException ex = new PaladinRpcException(-32000, "unauthorized", data, 401);
        assertEquals(-32000, ex.code());
        assertEquals(401, ex.httpStatus());
        assertTrue(ex.data().isPresent());
        assertEquals(data, ex.data().get());
    }

    @Test
    void rpcExceptionWithoutDataReportsEmpty() {
        PaladinRpcException ex = new PaladinRpcException(0, "bad gateway", null, 502);
        assertEquals(0, ex.code());
        assertEquals(502, ex.httpStatus());
        assertFalse(ex.data().isPresent());
    }
}
