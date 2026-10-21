import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Form, Button } from "antd";
import { expect, it, vi } from "vitest";
import { SAMLApplicationFields } from "./SAMLApplicationFields";

it("accepts pasted SP metadata and submits only the XML input", async () => {
  const submit = vi.fn();
  render(
    <Form onFinish={submit}>
      <SAMLApplicationFields />
      <Button htmlType="submit">Save</Button>
    </Form>,
  );
  fireEvent.change(screen.getByLabelText("SP metadata XML"), {
    target: { value: "<EntityDescriptor/>" },
  });
  fireEvent.click(screen.getByText("Save"));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({
      saml: { metadataXml: "<EntityDescriptor/>" },
    }),
  );
});
