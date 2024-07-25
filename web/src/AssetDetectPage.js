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
import {Link} from "react-router-dom";
import {Button, Progress, Table, Tag} from "antd";
import BaseListPage from "./BaseListPage";
import * as Setting from "./Setting";
import * as AssetBackend from "./backend/AssetBackend";
import i18next from "i18next";

const AssetStatusRunning = "Running";

class AssetDetectPage extends BaseListPage {
  constructor(props) {
    super(props);
    this.state = {
      ...this.state,
      selectedRows: {},
      detectionComplete: false,
    };
    this.handleCheckboxChange = this.handleCheckboxChange.bind(this);
    this.handleSelectedKeys = this.handleSelectedKeys.bind(this);
  }

  componentDidMount() {
    AssetBackend.deleteDetectedAssets()
      .then((res) => {
        if (res.status === "ok") {
          AssetBackend.DetectAssets()
            .then((res) => {
              if (res.status === "ok") {
                this.setState({detectionComplete: true});
                Setting.showMessage("success", i18next.t("asset:Asset detection is complete."));
              } else {
                Setting.showMessage("error", `Failed to detect Asset: ${res.msg}`);
              }
            })
            .catch(error => {
              Setting.showMessage("error", `Failed to detect Asset: ${res.msg}`);
            });

          this.assetDetectTimer = setInterval(() => {
            this.fetch({pagination: this.state.pagination, searchedColumn: this.state.searchedColumn, searchText: this.state.searchText, sortField: this.state.sortField, sortOrder: this.state.sortOrder}, true);
          }, 3000);
        } else {
          Setting.showMessage("error", `Failed to delete Asset: ${res.msg}`);
        }
      })
      .catch(error => {
        Setting.showMessage("error", `Asset failed to delete: ${error}`);
      });
  }

  componentWillUnmount() {
    clearInterval(this.assetDetectTimer);
    AssetBackend.deleteDetectedAssets();
  }

  detectAsset() {
    this.props.history.push({pathname: "/assets/detect", mode: "add"});
  }

  handleCheckboxChange(key) {
    this.setState(prevState => ({
      selectedRows: {
        ...prevState.selectedRows,
        [key]: !prevState.selectedRows[key],
      },
    }));
  }

  handleSelectedKeys() {
    const {selectedRows} = this.state;
    const selectedKeys = Object.keys(selectedRows).filter(key => selectedRows[key]);
    selectedKeys.forEach(key => {
      AssetBackend.addDetectedAsset(Setting.getRequestOrganization(this.props.account), key);
    });
    this.props.history.push({pathname: "/assets"});
  }

  renderTable(assets) {
    const columns = [
      {
        title: i18next.t("general:Organization"),
        dataIndex: "owner",
        key: "owner",
        width: "110px",
        sorter: true,
        ...this.getColumnSearchProps("owner"),
        render: (text, asset, index) => {
          return (
            <a target="_blank" rel="noreferrer" href={Setting.getMyProfileUrl(this.props.account).replace("/account", `/organizations/${text}`)}>
              {text}
            </a>
          );
        },
      },
      {
        title: i18next.t("general:Name"),
        dataIndex: "name",
        key: "name",
        width: "120px",
        sorter: true,
        ...this.getColumnSearchProps("name"),
        render: (text, record, index) => {
          return (
            <Link to={`/assets/${record.owner}/${record.name}`}>{text}</Link>
          );
        },
      },
      {
        title: i18next.t("general:Created time"),
        dataIndex: "createdTime",
        key: "createdTime",
        width: "160px",
        sorter: (a, b) => a.createdTime.localeCompare(b.createdTime),
        render: (text, record, index) => {
          return Setting.getFormattedDate(text);
        },
      },
      {
        title: i18next.t("general:Display name"),
        dataIndex: "displayName",
        key: "displayName",
        width: "150px",
        sorter: (a, b) => a.displayName.localeCompare(b.displayName),
      },
      {
        title: i18next.t("general:Category"),
        dataIndex: "category",
        key: "category",
        width: "100px",
        filterMultiple: false,
        filters: [
          {text: "Machine", value: "Machine"},
          {text: "Database", value: "Database"},
        ],
        sorter: true,
        render: (text, record, index) => {
          return <Tag color={text === "Machine" ? "blue" : "green"}>{text}</Tag>;
        },
      },
      {
        title: i18next.t("general:Type"),
        dataIndex: "type",
        key: "type",
        width: "100px",
        sorter: true,
        filterMultiple: false,
        filters: Setting.getMachineTypes().concat(Setting.getDatabaseTypes()).map((item) => {return {text: item, value: item};}),
      },
      {
        title: i18next.t("general:Endpoint"),
        dataIndex: "endpoint",
        key: "endpoint",
        width: "120px",
      },
      {
        title: i18next.t("general:Port"),
        dataIndex: "port",
        key: "port",
        width: "90px",
      },
      {
        title: i18next.t("general:Username"),
        dataIndex: "username",
        key: "username",
        width: "130px",
        sorter: (a, b) => a.username.localeCompare(b.username),
      },
      {
        title: i18next.t("general:Status"),
        dataIndex: "status",
        key: "status",
        width: "100px",
        render: (text, record, index) => {
          if (record.category !== "Machine") {
            return "";
          }
          return <Tag color={text === AssetStatusRunning ? "green" : "red"}>{text}</Tag>;
        },
      },
      {
        title: i18next.t("asset:CPU"),
        dataIndex: "cpuCurrent",
        key: "cpuCurrent",
        width: "150px",
        render: (text, record, index) => {
          if (record.status !== AssetStatusRunning || record.cpuTotal === 0) {
            return "";
          }

          return <Progress steps={20} size={"small"}
            percent={(text).toFixed(2)}
          />;
        },
      },
      {
        title: i18next.t("asset:Memory"),
        dataIndex: "memory",
        key: "memory",
        width: "150px",
        render: (text, record, index) => {
          if (record.status !== AssetStatusRunning || record.memTotal === 0) {
            return "";
          }

          return <Progress steps={20} size={"small"}
            percent={(record.memCurrent * 100 / record.memTotal).toFixed(2)}
          />;
        },
      },
      {
        title: i18next.t("asset:Disk"),
        dataIndex: "disk",
        key: "disk",
        width: "150px",
        render: (text, record, index) => {
          if (record.status !== AssetStatusRunning || record.diskTotal === 0) {
            return "";
          }

          return <Progress steps={20} size={"small"}
            percent={(record.diskCurrent * 100 / record.diskTotal).toFixed(2)}
          />;
        },
      },
      {
        title: i18next.t("general:Choose"),
        dataIndex: "Choose",
        key: "Choose",
        width: "160px",
        fixed: (Setting.isMobile()) ? "false" : "right",
        render: (text, record, index) => {
          return (
            <input
              type="checkbox"
              checked={!!this.state.selectedRows[record.name]}
              onChange={() => this.handleCheckboxChange(record.name)}
            />
          );
        },
      },
    ];

    const paginationProps = {
      pageSize: this.state.pagination.pageSize,
      total: this.state.pagination.total,
      showQuickJumper: true,
      showSizeChanger: true,
    };

    return (
      <div>
        <Table scroll={{x: "max-content"}} columns={columns} dataSource={assets} rowKey={(asset) => `${asset.owner}/${asset.name}`} size="middle" bordered pagination={paginationProps}
          title={() => (
            <div style={{display: "flex", justifyContent: "center", alignItems: "center"}}>
              <div style={{flex: 1, textAlign: "center"}}>
                {this.state.detectionComplete ? i18next.t("asset:Detection Complete") : i18next.t("asset:Detecting Assets...Please wait.")}
              </div>
              <Button type="primary" size="small" style={{marginLeft: "auto"}} disabled={!Setting.isAdminUser(this.props.account)} onClick={this.handleSelectedKeys}>
                {i18next.t("general:Add")}
              </Button>
            </div>
          )}
          loading={this.state.loading}
          onChange={this.handleTableChange}
        />
      </div>
    );
  }

  fetch = (params = {}, silent) => {
    let field = params.searchedColumn, value = params.searchText;
    const sortField = params.sortField, sortOrder = params.sortOrder;
    if (params.category) {
      field = "category";
      value = params.category;
    } else if (params.type) {
      field = "type";
      value = params.type;
    }
    if (!silent) {
      this.setState({loading: true});
    }
    AssetBackend.getDetectedAssets(Setting.getRequestOrganization(this.props.account), params.pagination.current, params.pagination.pageSize, field, value, sortField, sortOrder, silent)
      .then((res) => {
        if (!silent) {
          this.setState({loading: false});
        }
        if (res.status === "ok") {
          this.setState({
            data: res.data,
            pagination: {
              ...params.pagination,
              total: res.data2,
            },
            searchText: params.searchText,
            searchedColumn: params.searchedColumn,
          });
        } else {
          if (Setting.isResponseDenied(res)) {
            this.setState({
              isAuthorized: false,
            });
          } else {
            Setting.showMessage("error", res.msg);
          }
        }
      });
  };
}

export default AssetDetectPage;
