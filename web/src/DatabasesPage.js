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

import React, {useEffect, useState} from "react";
import * as Setting from "./Setting";
import i18next from "i18next";
import {Button, Result} from "antd";
import * as AssetBackend from "./backend/AssetBackend";

const DatabasesPage = (props) => {
  const {activeKey} = props;
  const [error, setError] = useState(null);

  useEffect(() => {
    AssetBackend.checkDbgate().then((res) => {
      if (res.status === "error") {
        setError(res.msg);
      }
    });
  }, []);

  const getHeight = () => {
    if (activeKey) {
      return "calc(100vh - 40px)";
    } else {
      return "100vh";
    }
  };

  if (error) {
    return <Result
      status="500"
      title="500"
      subTitle={<p>{error}</p>}
      extra={<a href="/assets"><Button type="primary">{i18next.t("general:Back")}</Button></a>}
    />;
  }

  return (
    <div>
      <iframe src={`${Setting.ServerUrl}/dbgate`} style={{width: "100%", height: getHeight()}} />
    </div>
  );
};

export default DatabasesPage;
