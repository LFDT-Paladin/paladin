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

package io.kaleido.paladin.pente.domain;

import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import org.junit.jupiter.api.Test;

public class PenteDomainTxIdForLogTest {

        @Test
        void testTxIdForLogConvertsBytes32IdToUuid() {
                String txId = "0x6fda3bc284c74ddab6029d20d4d63d3900000000000000000000000000000000";
                String logValue = PenteDomain.txIdForLog(txId);
                assertEquals(UUID.fromString("6fda3bc2-84c7-4dda-b602-9d20d4d63d39").toString(), logValue);
        }

        @Test
        void testTxIdForLogFallsBackToOriginalValueForInvalidHex() {
                String txId = "not-a-hex-id";
                assertEquals(txId, PenteDomain.txIdForLog(txId));
        }
}
