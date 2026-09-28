import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import dayjs from "dayjs";
import "dayjs/locale/zh-cn";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import "./styles.css";
import { theme } from "./theme";

// 日期控件的文案跟随系统语言（界面本来就是中文）
dayjs.locale("zh-cn");

const container = document.getElementById("root");
if (container) {
  createRoot(container).render(
    <StrictMode>
      <ConfigProvider theme={theme} locale={zhCN}>
        <AntApp>
          <App />
        </AntApp>
      </ConfigProvider>
    </StrictMode>,
  );
}
