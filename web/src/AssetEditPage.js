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
import {Button, Card, Col, Input, Row, Select, Switch} from "antd";
import * as AssetBackend from "./backend/AssetBackend";
import * as Setting from "./Setting";
import i18next from "i18next";
import ServiceTable from "./ServiceTable";
import RemoteAppTable from "./RemoteAppTable";

class AssetEditPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
      assetOwner: props.match.params.organizationName,
      assetName: props.match.params.assetName,
      asset: null,
      organizations: [],
      mode: props.location.mode !== undefined ? props.location.mode : "edit",
    };

    this.timer = null;
  }

  UNSAFE_componentWillMount() {
    this.getAsset();
    this.startTimer();
  }

  componentWillUnmount() {
    this.stopTimer();
  }

  getAsset(updateFromRemote = false) {
    AssetBackend.getAsset(this.props.account.owner, this.state.assetName)
      .then((res) => {
        if (res.status === "ok") {
          // Clone the fetched asset data using the spread operator
          const newAsset = {...res.data};

          if (!updateFromRemote && this.state.asset !== null) {
            newAsset.autoQuery = this.state.asset.autoQuery;

            // Update or add remote Apps to the asset data
            newAsset.remoteApps = newAsset.remoteApps.map((newApp, i) => {
              if (i < this.state.asset.remoteApps.length) {
                const oldApp = this.state.asset.remoteApps[i];
                return {
                  ...oldApp, // Preserve old attributes
                  ...newApp, // Override with new attributes
                };
              } else {
                return newApp; // Add new service
              }
            }).slice(0, this.state.asset.remoteApps.length);

            // Update or add services to the asset data
            newAsset.services = newAsset.services.map((newService, i) => {
              if (i < this.state.asset.services.length) {
                const oldService = this.state.asset.services[i];
                return {
                  ...oldService, // Preserve old attributes
                  ...newService, // Override with new attributes
                };
              } else {
                return newService; // Add new service
              }
            }).slice(0, this.state.asset.services.length);
          }

          this.setState({
            asset: newAsset,
          });
        } else {
          Setting.showMessage("error", `Failed to get asset: ${res.msg}`);
        }
      });
  }

  startTimer() {
    if (this.timer === null) {
      this.timer = window.setInterval(this.doTimer.bind(this), 3000);
    }
  }

  stopTimer() {
    if (this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  doTimer() {
    if (this.state.asset?.autoQuery) {
      this.getAsset(false);
    }
  }

  parseAssetField(key, value) {
    if (["port", "gatewayPort"].includes(key)) {
      value = Setting.myParseInt(value);
    }
    return value;
  }

  updateAssetField(key, value) {
    value = this.parseAssetField(key, value);

    const asset = this.state.asset;
    asset[key] = value;
    this.setState({
      asset: asset,
    });
  }

  getDefaultPort(key) {
    switch (key) {
    case "RDP":
      return 3389;
    case "VNC":
      return 5900;
    case "SSH":
      return 22;
    case "Telnet":
      return 23;
    case "MySQL":
      return 3306;
    case "Microsoft SQL Server":
      return 1433;
    case "Oracle":
      return 1521;
    case "PostgreSQL":
      return 5432;
    case "Redis":
      return 6379;
    case "MongoDB":
      return 27017;
    default:
      return 0;
    }
  }

  omitSetting(asset) {
    if (asset.category === "Machine") {
      asset.authType = "";
      asset.defaultDatabase = "";
      asset.databaseUrl = "";
      asset.useDatabaseUrl = false;
    } else if (asset.category === "Database") {
      asset.autoQuery = false;
      asset.isPermanent = false;
      asset.remoteApps = [];
      asset.services = [];
      asset.enableRemoteApp = false;
    }
    return asset;
  }

  renderAsset() {
    const {asset} = this.state;

    return (
      <Card size="small" title={
        <div>
          {this.state.mode === "add" ? i18next.t("asset:New Asset") : i18next.t("asset:Edit Asset")}&nbsp;&nbsp;&nbsp;&nbsp;
          <Button onClick={() => this.submitAssetEdit(false)}>{i18next.t("general:Save")}</Button>
          <Button style={{marginLeft: "20px"}} type="primary" onClick={() => this.submitAssetEdit(true)}>{i18next.t("general:Save & Exit")}</Button>
          {this.state.mode === "add" ? <Button style={{marginLeft: "20px"}} onClick={() => this.deleteAsset()}>{i18next.t("general:Cancel")}</Button> : null}
        </div>
      } style={{marginLeft: "5px"}} type="inner">
        <Row style={{marginTop: "10px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Organization"), i18next.t("general:Organization - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.owner} onChange={e => {
              this.updateAssetField("owner", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Name"), i18next.t("general:Name - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.name} onChange={e => {
              this.updateAssetField("name", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Display name"), i18next.t("general:Display name - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.displayName} onChange={e => {
              this.updateAssetField("displayName", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Category"), i18next.t("general:DisplayName - Category"))} :
          </Col>
          <Col span={22} >
            <Select virtual={false} style={{width: "100%"}} value={asset.category}
              options={[
                {label: "Machine", value: "Machine"},
                {label: "Database", value: "Database"},
              ].map((item) => Setting.getOption(item.label, item.value))}
              onChange={value => {
                this.updateAssetField("category", value);
                this.updateAssetField("type", "");
              }}
            />
          </Col>
        </Row>
        {
          asset.category === "Machine" &&
          <Row style={{marginTop: "20px"}} >
            <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
              {Setting.getLabel(i18next.t("general:Protocol"), i18next.t("general:Protocol - Tooltip"))} :
            </Col>
            <Col span={22} >
              <Select virtual={false} style={{width: "100%"}} value={asset.type}
                options={Setting.getMachineTypes().map((item) => Setting.getOption(item, item))}
                onChange={value => {
                  this.updateAssetField("type", value);
                  this.updateAssetField("port", this.getDefaultPort(value));
                }}
              />
            </Col>
          </Row>
        }
        {
          asset.category === "Database" &&
          <Row style={{marginTop: "20px"}} >
            <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
              {Setting.getLabel(i18next.t("general:Database type"), i18next.t("general:Database type - Tooltip"))} :
            </Col>
            <Col span={22} >
              <Select virtual={false} style={{width: "100%"}} value={asset.type}
                options={Setting.getDatabaseTypes().map((item) => Setting.getOption(item, item))}
                onChange={value => {
                  this.updateAssetField("type", value);
                  this.updateAssetField("port", this.getDefaultPort(value));
                }}
              />
            </Col>
          </Row>
        }
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("asset:Gatewat port"), i18next.t("asset:Gatewat port - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.gatewayPort} onChange={e => {
              this.updateAssetField("gatewayPort", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Endpoint"), i18next.t("general:Endpoint - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.endpoint} onChange={e => {
              this.updateAssetField("endpoint", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Port"), i18next.t("general:Port - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input
              value={asset.port}
              defaultValue={this.getDefaultPort(asset.protocol)}
              onChange={e => {
                this.updateAssetField("port", e.target.value);
              }}
            />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Username"), i18next.t("general:Username - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.username} onChange={e => {
              this.updateAssetField("username", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Password"), i18next.t("general:Password - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.password} onChange={e => {
              this.updateAssetField("password", e.target.value);
            }} />
          </Col>
        </Row>
        {asset.category === "Machine" && asset.type !== "SSH" && (
          <Row style={{marginTop: "20px"}} >
            <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
              {Setting.getLabel(i18next.t("asset:Enable SSH"), i18next.t("asset:Enable SSH - Tooltip"))} :
            </Col>
            <Col span={22} >
              <Switch checked={asset.enableSsh} onChange={checked => {
                this.updateAssetField("enableSsh", checked);
              }
              } />
            </Col>
          </Row>
        )}
        <Row style={{marginTop: "20px"}}>
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("asset:OS"), i18next.t("asset:OS - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Select virtual={false} style={{width: "100%"}} value={asset.os} onChange={value => {
              this.updateAssetField("os", value);
            }}
            options={[
              {value: "Windows", label: "Windows"},
              {value: "Linux", label: "Linux"},
            ].map(item => Setting.getOption(item.label, item.value))} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}}>
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general: Tag"), i18next.t("general: Tag - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.tags} onChange={e => {
              this.updateAssetField("tag", e.target.value);
            }
            } />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Language"), i18next.t("general:Language - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={asset.language} onChange={e => {
              this.updateAssetField("language", e.target.value);
            }} />
          </Col>
        </Row>
        {
          asset.category === "Machine" && (
            <div>
              <Row style={{marginTop: "20px"}} >
                <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
                  {Setting.getLabel(i18next.t("general:Auto query"), i18next.t("general:Auto query - Tooltip"))} :
                </Col>
                <Col span={22} >
                  <Switch checked={asset.autoQuery} onChange={checked => {
                    this.updateAssetField("autoQuery", checked);
                  }} />
                </Col>
              </Row>
              <Row style={{marginTop: "20px"}} >
                <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
                  {Setting.getLabel(i18next.t("general:Is Permanent"), i18next.t("application:Is Permanent - Tooltip"))} :
                </Col>
                <Col span={1} >
                  <Switch checked={asset.isPermanent} onChange={checked => {
                    this.updateAssetField("isPermanent", checked);
                  }} />
                </Col>
              </Row>
              {
                asset.type === "RDP" && (
                  <React.Fragment>
                    <Row style={{marginTop: "20px"}} >
                      <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
                        {Setting.getLabel(i18next.t("general:Enable Remote App"), i18next.t("general:Enable Remote App - Tooltip"))} :
                      </Col>
                      <Col span={22}>
                        <Switch checked={asset.enableRemoteApp} onChange={checked => {
                          if (checked && asset.remoteApps.length === 0) {
                            Setting.showMessage("error", i18next.t("asset:Cannot enable Remote App when Remote Apps are empty. Please add at least one Remote App in below table first, then enable again"));
                            return;
                          }
                          this.updateAssetField("enableRemoteApp", checked);
                        }} />
                      </Col>
                    </Row>
                    <Row style={{marginTop: "20px"}} >
                      <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2} >
                        {Setting.getLabel(i18next.t("general:Remote Apps"), i18next.t("general:Remote Apps - Tooltip"))} :
                      </Col>
                      <Col span={22} >
                        <RemoteAppTable title={"Remote Apps"} table={asset.remoteApps} onUpdateTable={(value) => {
                          this.updateAssetField("remoteApps", value);
                          if (value.length === 0) {
                            this.updateAssetField("enableRemoteApp", false);
                          }
                        }} />
                      </Col>
                    </Row>
                  </React.Fragment>
                )
              }
              <Row style={{marginTop: "20px"}} >
                <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2} >
                  {Setting.getLabel(i18next.t("general:Services"), i18next.t("general:Services - Tooltip"))} :
                </Col>
                <Col span={22} >
                  <ServiceTable title={"Services"} table={asset.services} onUpdateTable={(value) => {
                    this.updateAssetField("services", value);
                  }} />
                </Col>
              </Row>
            </div>
          )}
      </Card>
    );
  }

  submitAssetEdit(willExist) {
    const asset = Setting.deepCopy(this.omitSetting(this.state.asset));
    AssetBackend.updateAsset(this.state.asset.owner, this.state.assetName, asset)
      .then((res) => {
        if (res.status === "ok") {
          if (res.data) {
            Setting.showMessage("success", "Successfully saved");
            this.setState({
              assetName: this.state.asset.name,
            });
            if (willExist) {
              this.props.history.push("/assets");
            } else {
              this.props.history.push(`/assets/${this.state.asset.owner}/${encodeURIComponent(this.state.asset.name)}`);
            }
            // this.getAsset(true);
          } else {
            Setting.showMessage("error", "failed to save: server side failure");
            this.updateAssetField("name", this.state.assetName);
          }
        } else {
          Setting.showMessage("error", `failed to save: ${res.msg}`);
        }
      })
      .catch(error => {
        Setting.showMessage("error", `failed to save: ${error}`);
      });
  }

  deleteAsset() {
    AssetBackend.deleteAsset(this.state.asset)
      .then((res) => {
        if (res.status === "ok") {
          this.props.history.push("/assets");
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to delete")}: ${res.msg}`);
        }
      })
      .catch(error => {
        Setting.showMessage("error", `${i18next.t("general:Failed to connect to server")}: ${error}`);
      });
  }

  render() {
    return (
      <div>
        {
          this.state.asset !== null ? this.renderAsset() : null
        }
        <div style={{marginTop: "20px", marginLeft: "40px"}}>
          <Button size="large" onClick={() => this.submitAssetEdit(false)}>{i18next.t("general:Save")}</Button>
          <Button style={{marginLeft: "20px"}} type="primary" size="large" onClick={() => this.submitAssetEdit(true)}>{i18next.t("general:Save & Exit")}</Button>
          {this.state.mode === "add" ? <Button style={{marginLeft: "20px"}} size="large" onClick={() => this.deleteAsset()}>{i18next.t("general:Cancel")}</Button> : null}
        </div>
      </div>
    );
  }
}

export default AssetEditPage;
