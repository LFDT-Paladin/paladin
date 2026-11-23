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
package io.kaleido.paladin.loader;

import java.net.URL;
import java.net.URLClassLoader;
import java.util.List;

// Loads a plugin from its own jar, on top of the JDK, so the plugin runs on the libraries it was built with
// rather than core's.
class PluginClassLoader extends URLClassLoader {

    // Logging is configured once, by core, for the whole process. A plugin using its own copies of these
    // packages would log outside that configuration.
    private static final List<String> SHARED_PACKAGES = List.of(
            "org.apache.logging.log4j.",
            "org.slf4j.",
            "io.kaleido.paladin.logging."
    );

    static {
        registerAsParallelCapable();
    }

    private final ClassLoader core;

    PluginClassLoader(URL jar, ClassLoader core) {
        super(new URL[]{jar}, ClassLoader.getPlatformClassLoader());
        this.core = core;
    }

    @Override
    protected Class<?> loadClass(String name, boolean resolve) throws ClassNotFoundException {
        for (String prefix : SHARED_PACKAGES) {
            if (name.startsWith(prefix)) {
                return core.loadClass(name);
            }
        }
        return super.loadClass(name, resolve);
    }
}
