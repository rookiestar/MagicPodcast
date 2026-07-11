import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TagFormFields from "../TagFormFields";

vi.mock("@/components/ui/ColorPicker", () => ({
  default: ({
    value,
    disabled,
    onChange,
  }: {
    value: string;
    disabled?: boolean;
    onChange: (color: string) => void;
  }) => (
    <button
      type="button"
      disabled={disabled}
      onClick={() => onChange("#000000")}
    >
      颜色 {value}
    </button>
  ),
}));

function renderFields(overrides = {}) {
  const props = {
    name: "科技",
    color: "#2563eb",
    error: "",
    loading: false,
    onNameChange: vi.fn(),
    onColorChange: vi.fn(),
    ...overrides,
  };

  return {
    props,
    ...render(<TagFormFields {...props} />),
  };
}

describe("TagFormFields", () => {
  it("renders name, counter and errors", () => {
    renderFields({ error: "标签名称不能为空" });

    expect(screen.getByPlaceholderText("输入标签名称")).toHaveValue("科技");
    expect(screen.getByText("2/50")).toBeInTheDocument();
    expect(screen.getByText("标签名称不能为空")).toBeInTheDocument();
  });

  it("forwards name and color changes", () => {
    const { props } = renderFields();

    fireEvent.change(screen.getByPlaceholderText("输入标签名称"), {
      target: { value: "AI" },
    });
    fireEvent.click(screen.getByRole("button", { name: "颜色 #2563eb" }));

    expect(props.onNameChange).toHaveBeenCalledWith("AI");
    expect(props.onColorChange).toHaveBeenCalledWith("#000000");
  });

  it("disables controls while loading", () => {
    renderFields({ loading: true });

    expect(screen.getByPlaceholderText("输入标签名称")).toBeDisabled();
    expect(screen.getByRole("button", { name: "颜色 #2563eb" })).toBeDisabled();
  });
});
