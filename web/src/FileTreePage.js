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

import React from "react";
import {Button, Col, Result, Row, Spin} from "antd";
import FileTree from "./FileTree";
import i18next from "i18next";
import * as Setting from "./Setting";
import * as SessionBackend from "./backend/SessionBackend";
import * as FileBackend from "./backend/FileBackend";

class FileTreePage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
      assetOwner: props.match.params.organizationName,
      assetName: props.match.params.assetName,
      session: null,
      store: null,
      spinning: false,
      key: "/",
      msg: "",
    };
  }

  componentDidMount() {
    this.addAssetTunnel();
  }

  addAssetTunnel() {
    const {assetOwner, assetName} = this.state;

    this.setState({
      spinning: true,
    });

    SessionBackend.addAssetTunnel(`${assetOwner}/${assetName}`, "file").then((res) => {
      this.setState({
        spinning: false,
      });

      if (res.status === "ok") {
        const session = res.data;
        this.setState({
          session: session,
        }, () => {
          this.getStore();
        });
      } else {
        this.setState({
          msg: res.msg,
        });
        Setting.showMessage("error", "Failed to connect: " + res.msg);
      }
    });
  }

  getStore() {
    const {session, key} = this.state;
    FileBackend.getFiles(`${session.owner}/${session.name}`, key, "store").then((res) => {
      if (res.status === "ok") {
        const store = res.data;
        this.setState({
          store: store,
        });
      } else {
        Setting.showMessage("error", "Failed to get store: " + res.msg);
      }
    });
  }

  render() {
    const {store, spinning, msg} = this.state;

    if (msg !== "") {
      return (
        <div className="App">
          <Result
            status="error"
            title={i18next.t("general:Error")}
            subTitle={msg}
            extra={
              <Button type="primary" onClick={() => this.props.history.push("/assets")}>
                {i18next.t("general:Back Assets")}
              </Button>
            }
          />
        </div>
      );
    }

    if (this.state.store === null) {
      return (
        <div className="App">
          <Spin size="large" tip={i18next.t("general:Loading...")} style={{paddingTop: "10%"}} spinning={spinning} />
        </div>
      );
    }

    return (
      <div>
        <Row>
          <Col span={24}>
            <FileTree account={this.props.account} store={store} session={this.state.session}
              onUpdateStore={(store) => {
                this.setState({
                  store: store,
                });
              }}
              onRefresh={() => {
                this.getStore();
              }} />
          </Col>
        </Row>
      </div>
    );
  }
}

export default FileTreePage;
