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
package org.lfdt.paladin.sdk.core.abi;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import org.junit.jupiter.api.Test;

/** JSON token mapping for the ABI enums. */
class AbiEnumsTest {

  @Test
  void stateMutabilityMapsEveryToken() {
    for (final StateMutability mutability : StateMutability.values()) {
      assertEquals(mutability, StateMutability.fromJson(mutability.jsonValue()));
      assertEquals(mutability, StateMutability.fromJson(mutability.jsonValue().toUpperCase()));
    }
    assertEquals("nonpayable", StateMutability.NONPAYABLE.jsonValue());
  }

  @Test
  void stateMutabilityRejectsUnknownTokens() {
    assertThrows(IllegalArgumentException.class, () -> StateMutability.fromJson("constant"));
    assertThrows(IllegalArgumentException.class, () -> StateMutability.fromJson(null));
  }

  @Test
  void entryTypeMapsEveryToken() {
    for (final EntryType type : EntryType.values()) {
      assertEquals(type, EntryType.fromJson(type.jsonValue()));
      assertEquals(type, EntryType.fromJson(type.jsonValue().toUpperCase()));
    }
    assertEquals("constructor", EntryType.CONSTRUCTOR.jsonValue());
  }

  @Test
  void entryTypeRejectsUnknownTokens() {
    assertThrows(IllegalArgumentException.class, () -> EntryType.fromJson("modifier"));
    assertThrows(IllegalArgumentException.class, () -> EntryType.fromJson(null));
  }
}
