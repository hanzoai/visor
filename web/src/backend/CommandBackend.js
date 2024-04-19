// Copyright 2024 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import * as Setting from "../Setting";

export function getCommands(owner, page = "", pageSize = "", field = "", value = "", sortField = "", sortOrder = "") {
  return fetch(`${Setting.ServerUrl}/api/get-commands?owner=${owner}&p=${page}&pageSize=${pageSize}&field=${field}&value=${value}&sortField=${sortField}&sortOrder=${sortOrder}`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function getCommand(owner, name) {
  return fetch(`${Setting.ServerUrl}/api/get-command?id=${owner}/${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function updateCommand(owner, name, command) {
  const newCommand = Setting.deepCopy(command);
  return fetch(`${Setting.ServerUrl}/api/update-command?id=${owner}/${encodeURIComponent(name)}`, {
    method: "POST",
    credentials: "include",
    body: JSON.stringify(newCommand),
  }).then(res => res.json());
}

export function addCommand(command) {
  const newCommand = Setting.deepCopy(command);
  return fetch(`${Setting.ServerUrl}/api/add-command`, {
    method: "POST",
    credentials: "include",
    body: JSON.stringify(newCommand),
  }).then(res => res.json());
}

export function deleteCommand(command) {
  const newCommand = Setting.deepCopy(command);
  return fetch(`${Setting.ServerUrl}/api/delete-command`, {
    method: "POST",
    credentials: "include",
    body: JSON.stringify(newCommand),
  }).then(res => res.json());
}

export function execCommand(owner, name, assetName, onMessage, onError) {
  const eventSource = new EventSource(`${Setting.ServerUrl}/api/exec-command?id=${owner}/${encodeURIComponent(name)}&assetId=${owner}/${encodeURIComponent(assetName)}`,
    {
      withCredentials: true,
    });

  eventSource.addEventListener("message", (e) => {
    onMessage(e.data);
  });

  eventSource.addEventListener("myerror", (e) => {
    onError(e.data);
    eventSource.close();
  });

  eventSource.addEventListener("end", (e) => {
    eventSource.close();
  });
}
