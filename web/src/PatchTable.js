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

import React from "react";
import {Col, Row, Select, Table} from "antd";
import * as Setting from "./Setting";
import i18next from "i18next";

class PatchTable extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
    };
  }

  updateTable(table) {
    this.props.onUpdateTable(table);
  }

  parseField(key, value) {
    if (["no", "port", "processId"].includes(key)) {
      value = Setting.myParseInt(value);
    }
    return value;
  }

  updateField(table, index, key, value) {
    value = this.parseField(key, value);

    table[index][key] = value;
    this.updateTable(table);
  }

  handleAction(table, index, action) {
    this.updateField(table, index, "ExceptedStatus", action);
  }

  deleteRow(table, index) {
    table = Setting.deleteRow(table, index);
    this.updateTable(table);
  }

  renderTable(table) {
    const columns = [
      {
        title: i18next.t("general:Name"),
        dataIndex: "name",
        key: "name",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "name", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:Category"),
        dataIndex: "category",
        key: "category",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "category", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:Title"),
        dataIndex: "title",
        key: "title",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "title", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:Url"),
        dataIndex: "url",
        key: "url",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "url", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:Size"),
        dataIndex: "size",
        key: "size",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "size", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:Status"),
        dataIndex: "status",
        key: "status",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "status", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:InstallTime"),
        dataIndex: "installTime",
        key: "installTime",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "installTime", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:Message"),
        dataIndex: "message",
        key: "message",
        width: "100px",
        render: (text, record, index) => {
          return (
            <a-input
              value={text}
              onChange={(e) => {
                this.updateField(table, index, "message", e.target.value);
              }}
            />
          );
        },
      },
      {
        title: i18next.t("general:ExpectedStatus"),
        key: "expectedStatus",
        width: "200px",
        render: (text, record, index) => {
          return (
            <Select
              value={record.status}
              style={{width: 120}}
              onChange={(value) => this.handleAction(table, index, value)}
            >
              <Select.Option value="">{i18next.t("general:None")}</Select.Option>
              <Select.Option value="Install">{i18next.t("asset:Install")}</Select.Option>
              <Select.Option value="Uninstall">{i18next.t("asset:Uninstall")}</Select.Option>
            </Select>
          );
        },
      },
    ];

    return (
      <Table
        rowKey="index"
        columns={columns}
        dataSource={table}
        size="middle"
        bordered
        pagination={false}
        title={() => (
          <div>
            {this.props.title}&nbsp;&nbsp;&nbsp;&nbsp;
          </div>
        )}
      />
    );
  }

  render() {
    return (
      <div>
        <Row style={{marginTop: "20px"}}>
          <Col span={24}>
            {this.renderTable(this.props.table)}
          </Col>
        </Row>
      </div>
    );
  }
}

export default PatchTable;
