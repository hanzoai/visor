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
import {Button, Card, Col, Input, List, Row, Select} from "antd";
import * as CommandBackend from "./backend/CommandBackend";
import * as Setting from "./Setting";
import i18next from "i18next";
import * as AssetBackend from "./backend/AssetBackend";

class CommandEditPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
      command: null,
      organizations: [],
      assets: [],
      owner: props.account.owner,
      commandName: props.match.params.commandName !== undefined ? props.match.params.commandName : "",
      mode: props.location.mode !== undefined ? props.location.mode : "edit",
      results: [],
    };
    this.scrollState = {};
  }

  UNSAFE_componentWillMount() {
    this.getCommand();
    this.getAssets();
  }

  componentDidUpdate(prevProps, prevState) {
    const scrollThreshold = 5;
    this.state.results.forEach((result) => {
      const textarea = document.getElementById(`textarea-${result.title}`);
      if (textarea !== null) {
        if (this.scrollState[result.title] !== undefined && Math.abs(textarea.scrollTop - this.scrollState[result.title]) > scrollThreshold) {
          return;
        }
        textarea.scrollTop = textarea.scrollHeight;
        this.scrollState[result.title] = textarea.scrollTop;
      }
    });
  }

  getCommand() {
    CommandBackend.getCommand(this.props.account.owner, this.state.commandName)
      .then((res) => {
        if (res.status === "ok") {
          this.setState({
            command: res.data,
            results: res.data.assets.map(asset => {
              return {title: asset, text: ""};
            }),
          });
        } else {
          Setting.showMessage("error", `Failed to get command: ${res.msg}`);
        }
      });
  }

  getAssets() {
    AssetBackend.getAssets(this.state.owner).then((res) => {
      if (res.status === "ok") {
        this.setState({
          assets: res.data,
        });
      } else {
        Setting.showMessage("error", `Failed to get assets: ${res.msg}`);
      }
    });
  }

  parseCommandField(key, value) {
    // if ([""].includes(key)) {
    //   value = Setting.myParseInt(value);
    // }

    return value;
  }

  updateCommandField(key, value) {
    value = this.parseCommandField(key, value);

    const command = this.state.command;
    command[key] = value;
    this.setState({
      command: command,
    });
  }

  renderCommand() {
    const {command} = this.state;

    return (
      <Card size="small" title={
        <div>
          {this.state.mode === "add" ? i18next.t("command:New Command") : i18next.t("command:Edit Command")}&nbsp;&nbsp;&nbsp;&nbsp;
          <Button onClick={() => this.submitCommandEdit(false)}>{i18next.t("general:Save")}</Button>
          <Button style={{marginLeft: "20px"}} type="primary" onClick={() => this.submitCommandEdit(true)}>{i18next.t("general:Save & Exit")}</Button>
          {this.state.mode === "add" ? <Button style={{marginLeft: "20px"}} onClick={() => this.deleteCommand()}>{i18next.t("general:Cancel")}</Button> : null}
        </div>
      } style={{marginLeft: "5px"}} type="inner">
        <Row style={{marginTop: "10px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Organization"), i18next.t("general:Organization - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={command.owner} onChange={e => {
              this.updateCommandField("owner", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Name"), i18next.t("general:Name - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={command.name} onChange={e => {
              this.updateCommandField("name", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Display name"), i18next.t("general:Display name - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input value={command.displayName} onChange={e => {
              this.updateCommandField("displayName", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("command:Command"), i18next.t("command:Command - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Input.TextArea value={command.command} rows={4} onChange={e => {
              this.updateCommandField("command", e.target.value);
            }} />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("general:Assets"), i18next.t("general:Assets - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Select virtual={false} style={{width: "100%"}} mode="multiple" value={command.assets}
              options={this.state.assets.filter(asset => asset.type === "SSH").map(asset => Setting.getOption(asset.displayName, asset.name))}
              onChange={value => {
                const results = [];
                value.forEach((asset) => {
                  if (this.state.results.find(result => result.title === asset) !== undefined) {
                    results.push(this.state.results.find(result => result.title === asset));
                  } else {
                    results.push({title: asset, text: ""});
                  }
                });
                this.updateCommandField("assets", value);
                this.setState({
                  results: results,
                });
              }}
            />
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("command:Run"), i18next.t("command:Run - Tooltip"))} :
          </Col>
          <Col span={22} >
            <Button type="primary" style={{width: 180}} onClick={() => {
              command.assets.forEach((asset) => {
                CommandBackend.execCommand(this.state.owner, this.state.commandName, asset, (data) => {
                  const jsonData = JSON.parse(data);
                  if (jsonData.text === "") {
                    jsonData.text = "\n";
                  }
                  const results = this.state.results;
                  if (this.state.results.find(result => result.title === asset) === undefined) {
                    results.push({title: asset, text: jsonData.text});
                  } else {
                    results.find(result => result.title === asset).text += jsonData.text + "\n";
                  }
                  this.setState({
                    results: results,
                  });
                }, (error) => {
                  const results = this.state.results;
                  if (this.state.results.find(result => result.title === asset) === undefined) {
                    results.push({title: asset, text: error});
                  } else {
                    results.find(result => result.title === asset).text += error;
                  }
                  this.setState({
                    results: results,
                  });
                });
              }
              );
            }}>
              {i18next.t("command:Run All")}
            </Button>
          </Col>
        </Row>
        <Row style={{marginTop: "20px"}} >
          <Col style={{marginTop: "5px"}} span={(Setting.isMobile()) ? 22 : 2}>
            {Setting.getLabel(i18next.t("command:Output"), i18next.t("command:Output - Tooltip"))} :
          </Col>
          <Col span={22} >
            <List
              grid={{
                gutter: 16,
                column: 2,
              }}
              dataSource={this.state.results}
              renderItem={(item, index) => (
                <List.Item>
                  <Card title={item.title} size="small" extra={
                    <Button type="primary" onClick={() => {
                      CommandBackend.execCommand(this.state.owner, this.state.commandName, item.title, (data) => {
                        const jsonData = JSON.parse(data);
                        if (jsonData.text === "") {
                          jsonData.text = "\n";
                        }
                        const results = this.state.results;
                        if (this.state.results.find(result => result.title === item.title) === undefined) {
                          results.push({title: item.title, text: jsonData.text});
                        } else {
                          results.find(result => result.title === item.title).text += jsonData.text + "\n";
                        }
                        this.setState({
                          results: results,
                        });
                      }, (error) => {
                        const results = this.state.results;
                        if (this.state.results.find(result => result.title === item.title) === undefined) {
                          results.push({title: item.title, text: error});
                        } else {
                          results.find(result => result.title === item.title).text += error;
                        }
                        this.setState({
                          results: results,
                        });
                      });
                    }}>
                      {i18next.t("command:Run")}
                    </Button>
                  }>
                    <Input.TextArea id={`textarea-${item.title}`} value={item.text} rows={8} readOnly />
                  </Card>
                </List.Item>
              )}
            />
          </Col>
        </Row>
      </Card>
    );
  }

  submitCommandEdit(willExist) {
    const command = Setting.deepCopy(this.state.command);
    CommandBackend.updateCommand(this.state.owner, this.state.commandName, command)
      .then((res) => {
        if (res.status === "ok") {
          if (res.data) {
            Setting.showMessage("success", "Successfully saved");
            this.setState({
              commandName: this.state.command.name,
            });
            if (willExist) {
              this.props.history.push("/commands");
            } else {
              this.props.history.push(`/commands/${this.state.command.owner}/${encodeURIComponent(this.state.command.name)}`);
            }
          } else {
            Setting.showMessage("error", "failed to save: server side failure");
            this.updateCommandField("name", this.state.commandName);
          }
        } else {
          Setting.showMessage("error", `failed to save: ${res.msg}`);
        }
      })
      .catch(error => {
        Setting.showMessage("error", `failed to save: ${error}`);
      });
  }

  deleteCommand() {
    CommandBackend.deleteCommand(this.state.command)
      .then((res) => {
        if (res.status === "ok") {
          this.props.history.push("/commands");
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
          this.state.command !== null ? this.renderCommand() : null
        }
        <div style={{marginTop: "20px", marginLeft: "40px"}}>
          <Button size="large" onClick={() => this.submitCommandEdit(false)}>{i18next.t("general:Save")}</Button>
          <Button style={{marginLeft: "20px"}} type="primary" size="large" onClick={() => this.submitCommandEdit(true)}>{i18next.t("general:Save & Exit")}</Button>
          {this.state.mode === "add" ? <Button style={{marginLeft: "20px"}} size="large" onClick={() => this.deleteCommand()}>{i18next.t("general:Cancel")}</Button> : null}
        </div>
      </div>
    );
  }
}

export default CommandEditPage;
