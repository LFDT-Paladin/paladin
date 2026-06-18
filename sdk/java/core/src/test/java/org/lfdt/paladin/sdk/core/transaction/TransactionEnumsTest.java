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

package org.lfdt.paladin.sdk.core.transaction;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class TransactionEnumsTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void transactionTypeSerializesToLowerCaseToken() throws Exception {
        assertEquals("\"private\"", MAPPER.writeValueAsString(TransactionType.PRIVATE));
        assertEquals("\"public\"", MAPPER.writeValueAsString(TransactionType.PUBLIC));
    }

    @Test
    void transactionTypeParsesCaseInsensitively() throws Exception {
        assertEquals(TransactionType.PRIVATE, MAPPER.readValue("\"Private\"", TransactionType.class));
        assertEquals(TransactionType.PUBLIC, MAPPER.readValue("\"PUBLIC\"", TransactionType.class));
    }

    @Test
    void transactionTypeRejectsUnknownToken() {
        assertThrows(Exception.class, () -> MAPPER.readValue("\"hybrid\"", TransactionType.class));
    }

    @Test
    void submitModeSerializesToLowerCaseToken() throws Exception {
        assertEquals("\"auto\"", MAPPER.writeValueAsString(SubmitMode.AUTO));
        assertEquals("\"external\"", MAPPER.writeValueAsString(SubmitMode.EXTERNAL));
        assertEquals("\"call\"", MAPPER.writeValueAsString(SubmitMode.CALL));
        assertEquals("\"prepare\"", MAPPER.writeValueAsString(SubmitMode.PREPARE));
    }

    @Test
    void submitModeParsesCaseInsensitively() throws Exception {
        assertEquals(SubmitMode.AUTO, MAPPER.readValue("\"AUTO\"", SubmitMode.class));
        assertEquals(SubmitMode.PREPARE, MAPPER.readValue("\"Prepare\"", SubmitMode.class));
    }

    @Test
    void submitModeRejectsUnknownToken() {
        assertThrows(Exception.class, () -> MAPPER.readValue("\"manual\"", SubmitMode.class));
    }
}
