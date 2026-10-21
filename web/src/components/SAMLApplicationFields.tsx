import type { SAMLConfiguration } from "../lib/api";
import { useState } from "react";
import {
  Alert,
  Button,
  Descriptions,
  Form,
  Input,
  Upload,
  Typography,
} from "antd";
import { useI18n } from "../lib/i18n";

export function SAMLApplicationFields({
  summary,
}: {
  summary?: SAMLConfiguration;
}) {
  const { t } = useI18n();
  const form = Form.useFormInstance();
  const [fileName, setFileName] = useState("");
  const [fileError, setFileError] = useState(false);
  return (
    <>
      <Typography.Paragraph>
        <a href="/saml/metadata" target="_blank" rel="noreferrer">
          {t("samlIdpMetadata")}
        </a>
      </Typography.Paragraph>
      <Alert
        type="info"
        showIcon
        title={t("samlMetadataHint")}
        className="form-alert"
      />
      <Upload
        accept=".xml,application/xml,text/xml"
        showUploadList={false}
        beforeUpload={async (file) => {
          setFileError(false);
          if (file.size > 256 * 1024) {
            setFileError(true);
            return Upload.LIST_IGNORE;
          }
          try {
            const text = await file.text();
            form.setFieldValue(["saml", "metadataXml"], text);
            setFileName(file.name);
          } catch {
            setFileError(true);
          }
          return Upload.LIST_IGNORE;
        }}
      >
        <Button>{t("samlUploadMetadata")}</Button>
      </Upload>
      {fileName && <Typography.Paragraph>{fileName}</Typography.Paragraph>}
      {fileError && <Alert type="error" title={t("samlMetadataSize")} />}
      <Form.Item
        name={["saml", "metadataXml"]}
        label={t("samlMetadataXml")}
        preserve={false}
        rules={[
          { required: true, message: t("required") },
          {
            validator: (_, value) =>
              !value || new TextEncoder().encode(value).length <= 256 * 1024
                ? Promise.resolve()
                : Promise.reject(new Error(t("samlMetadataSize"))),
          },
        ]}
      >
        <Input.TextArea rows={9} spellCheck={false} />
      </Form.Item>
      {summary && (
        <Descriptions
          column={1}
          size="small"
          items={[
            {
              key: "entity",
              label: "SP Entity ID",
              children: summary.entityId,
            },
            { key: "acs", label: "ACS URL", children: summary.acsUrl },
            {
              key: "certificate",
              label: t("samlSigningCertificate"),
              children: (
                <Typography.Paragraph code copyable>
                  {summary.signingCertificate}
                </Typography.Paragraph>
              ),
            },
          ]}
        />
      )}
    </>
  );
}
