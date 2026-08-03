// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { Box, useMediaQuery, useTheme } from "@mui/material";
import {
  createBrowserRouter,
  Navigate,
  Outlet,
  RouteObject,
  RouterProvider,
} from "react-router-dom";
import { Header } from "./components/Header";
import { Navigation } from "./components/Navigation";
import { AppRoutes } from "./routes";
import { getBasePath } from "./utils";
import { Domains } from "./views/Domains";
import { DomainContract } from "./views/DomainContract";
import { EventListenerEntry } from "./views/EventListenerEntry";
import { EventListeners } from "./views/EventListeners";
import { Keys } from "./views/Keys";
import { PrivacyGroupEntry } from "./views/PrivacyGroupEntry";
import { PrivacyGroupListenerEntry } from "./views/PrivacyGroupListenerEntry";
import { PrivacyGroupListeners } from "./views/PrivacyGroupListeners";
import { PrivacyGroupMessageEntry } from "./views/PrivacyGroupMessageEntry";
import { PrivacyGroupMessages } from "./views/PrivacyGroupMessages";
import { PrivacyGroups } from "./views/PrivacyGroups";
import { ReceiptListenerEntry } from "./views/ReceiptListenerEntry";
import { ReceiptListeners } from "./views/ReceiptListeners";
import { Registries } from "./views/Registries";
import { RegistryEntry } from "./views/RegistryEntry";
import { MessageEntry } from "./views/ReliableMessageEntry";
import { StateEntry } from "./views/StateEntry";
import { States } from "./views/States";
import { Submissions } from "./views/Submissions";
import { TransactionEntry } from "./views/TransactionEntry";
import { Transactions } from "./views/Transactions";
import { TransportConnections } from "./views/TransportsConnections";
import { TransportMessages } from "./views/TransportMessages";

function AppLayout() {
  const theme = useTheme();
  const lessThanLarge = useMediaQuery(theme.breakpoints.down("lg"));

  return (
    <>
      {lessThanLarge && <Header />}
      <Box sx={{ display: "flex" }}>
        <Navigation />
        <Box sx={{ flexGrow: 1, maxWidth: "100vw", minWidth: 0 }}>
          <Outlet />
        </Box>
      </Box>
    </>
  );
}

export function getAllRoutes(): RouteObject[] {
  return [
    { path: AppRoutes.Transactions, element: <Transactions /> },
    { path: AppRoutes.Submissions, element: <Submissions /> },
    { path: AppRoutes.Transaction, element: <TransactionEntry /> },
    { path: AppRoutes.Keys, element: <Keys /> },
    { path: AppRoutes.Registries, element: <Registries /> },
    { path: AppRoutes.Domains, element: <Domains /> },
    { path: AppRoutes.DomainContract, element: <DomainContract /> },
    { path: AppRoutes.PrivacyGroups, element: <PrivacyGroups /> },
    { path: AppRoutes.PrivacyGroupMessages, element: <PrivacyGroupMessages /> },
    { path: AppRoutes.PrivacyGroupListeners, element: <PrivacyGroupListeners /> },
    { path: AppRoutes.PrivacyGroup, element: <PrivacyGroupEntry /> },
    { path: AppRoutes.PrivacyGroupMessageEntry, element: <PrivacyGroupMessageEntry /> },
    { path: AppRoutes.PrivacyGroupListenerEntry, element: <PrivacyGroupListenerEntry /> },
    { path: AppRoutes.States, element: <States /> },
    { path: AppRoutes.State, element: <StateEntry /> },
    { path: AppRoutes.RegistryEntry, element: <RegistryEntry /> },
    { path: AppRoutes.ReliableMessage, element: <MessageEntry /> },
    { path: AppRoutes.TransportConnections, element: <TransportConnections /> },
    { path: AppRoutes.TransportMessages, element: <TransportMessages /> },
    { path: AppRoutes.EventListeners, element: <EventListeners /> },
    { path: AppRoutes.EventListenerEntry, element: <EventListenerEntry /> },
    { path: AppRoutes.ReceiptListeners, element: <ReceiptListeners /> },
    { path: AppRoutes.ReceiptListenerEntry, element: <ReceiptListenerEntry /> },
    { path: "*", element: <Navigate to={AppRoutes.Transactions} replace /> },
  ];
}

const router = createBrowserRouter(
  [
    {
      path: "/",
      element: <AppLayout />,
      children: getAllRoutes(),
    },
  ],
  {
    basename: getBasePath(),
    future: {
      v7_relativeSplatPath: true,
    },
  }
);

export const Router = () => {
  return <RouterProvider router={router} />;
};
