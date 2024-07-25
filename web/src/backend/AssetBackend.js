// Copyright 2023 The casbin Authors. All Rights Reserved.
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

export function getAssets(owner, page = "", pageSize = "", field = "", value = "", sortField = "", sortOrder = "", silent = false) {
  return fetch(`${Setting.ServerUrl}/api/get-assets?owner=${owner}&p=${page}&pageSize=${pageSize}&field=${field}&value=${value}&sortField=${sortField}&sortOrder=${sortOrder}&silent=${silent}`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function getAsset(owner, name) {
  return fetch(`${Setting.ServerUrl}/api/get-asset?id=${owner}/${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function updateAsset(owner, name, asset) {
  const newAsset = Setting.deepCopy(asset);
  return fetch(`${Setting.ServerUrl}/api/update-asset?id=${owner}/${encodeURIComponent(name)}`, {
    method: "POST",
    credentials: "include",
    body: JSON.stringify(newAsset),
  }).then(res => res.json());
}

export function addAsset(asset) {
  const newAsset = Setting.deepCopy(asset);
  return fetch(`${Setting.ServerUrl}/api/add-asset`, {
    method: "POST",
    credentials: "include",
    body: JSON.stringify(newAsset),
  }).then(res => res.json());
}

export function deleteAsset(asset) {
  const newAsset = Setting.deepCopy(asset);
  return fetch(`${Setting.ServerUrl}/api/delete-asset`, {
    method: "POST",
    credentials: "include",
    body: JSON.stringify(newAsset),
  }).then(res => res.json());
}

export function checkDbgate() {
  return fetch(`${Setting.ServerUrl}/dbgate`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function RefreshAssetStatus() {
  return fetch(`${Setting.ServerUrl}/api/refresh-asset-status`, {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}

export function DetectAssets() {
  return fetch(`${Setting.ServerUrl}/api/detect-assets`, {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}

export function getDetectedAssets(owner, page = "", pageSize = "", field = "", value = "", sortField = "", sortOrder = "", silent = false) {
  return fetch(`${Setting.ServerUrl}/api/get-detected-assets?owner=${owner}&p=${page}&pageSize=${pageSize}&field=${field}&value=${value}&sortField=${sortField}&sortOrder=${sortOrder}&silent=${silent}`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function addDetectedAsset(owner, name) {
  return fetch(`${Setting.ServerUrl}/api/add-detected-asset?owner=${owner}&name=${name}`, {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}

export function deleteDetectedAssets() {
  return fetch(`${Setting.ServerUrl}/api/delete-detected-assets`, {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}
