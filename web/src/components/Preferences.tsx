import { Select, Space } from "antd";
import { GlobalOutlined, BulbOutlined } from "@ant-design/icons";
import { createContext, useContext } from "react";
import { useI18n } from "../lib/i18n";
export type Mode = "light" | "dark" | "system";
export const ThemeContext = createContext<{
  mode: Mode;
  setMode: (mode: Mode) => void;
}>({ mode: "system", setMode: () => {} });
export function Preferences() {
  const { t, language, setLanguage } = useI18n();
  const { mode, setMode } = useContext(ThemeContext);
  return (
    <Space size={8}>
      <Select
        aria-label={t("language")}
        variant="borderless"
        suffixIcon={<GlobalOutlined />}
        value={language}
        onChange={setLanguage}
        options={[
          { value: "en", label: "English" },
          { value: "zh-CN", label: "中文" },
        ]}
      />
      <Select
        aria-label={t("theme")}
        variant="borderless"
        suffixIcon={<BulbOutlined />}
        value={mode}
        onChange={setMode}
        options={(["light", "dark", "system"] as const).map((v) => ({
          value: v,
          label: t(v),
        }))}
      />
    </Space>
  );
}
