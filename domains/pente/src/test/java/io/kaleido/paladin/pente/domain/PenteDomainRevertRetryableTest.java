/*
 * Copyright © 2024 Kaleido, Inc.
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

package io.kaleido.paladin.pente.domain;

import com.google.protobuf.ByteString;
import io.kaleido.paladin.toolkit.IsBaseLedgerRevertRetryableRequest;
import org.junit.jupiter.api.Test;

import java.util.HexFormat;
import java.util.concurrent.ExecutionException;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

public class PenteDomainRevertRetryableTest {

    @Test
    void unknownSelectorReturnsEmptyDecodedReason() throws ExecutionException, InterruptedException {
        var pente = new PenteDomain("", "");

        var revertData = new byte[]{
                (byte) 0x08, (byte) 0xc3, (byte) 0x79, (byte) 0xa0, // Error(string) selector (not retryable for Pente)
                0x01, 0x02, 0x03, 0x04
        };

        var res = pente.isBaseLedgerRevertRetryable(IsBaseLedgerRevertRetryableRequest.newBuilder()
                .setRevertData(ByteString.copyFrom(revertData))
                .build()).get();

        assertFalse(res.getRetryable());
        assertEquals("", res.getDecodedReason());
    }

    @Test
    void knownRetryableSelectorDecodesReason() throws ExecutionException, InterruptedException {
        var pente = new PenteDomain("", "");

        var selector = new byte[]{(byte) 0xa8, (byte) 0x0f, (byte) 0x89, (byte) 0xf4}; // PenteInputNotAvailable
        var input = new byte[32];
        input[31] = 0x2a;
        var revertData = append(selector, input);

        var res = pente.isBaseLedgerRevertRetryable(IsBaseLedgerRevertRetryableRequest.newBuilder()
                .setRevertData(ByteString.copyFrom(revertData))
                .build()).get();

        assertTrue(res.getRetryable());
        assertEquals(
                "PenteInputNotAvailable(input=0x" + HexFormat.of().formatHex(input) + ")",
                res.getDecodedReason()
        );
    }

    private static byte[] append(byte[] a, byte[] b) {
        var out = new byte[a.length + b.length];
        System.arraycopy(a, 0, out, 0, a.length);
        System.arraycopy(b, 0, out, a.length, b.length);
        return out;
    }
}
