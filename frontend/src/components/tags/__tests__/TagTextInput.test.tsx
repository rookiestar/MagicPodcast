import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import TagTextInput from "../TagTextInput";

function renderInput(overrides = {}) {
  const props = {
    value: "",
    placeholder: "输入标签",
    disabled: false,
    onChangeValue: vi.fn(),
    onKeyDown: vi.fn(),
    onBlur: vi.fn(),
    onFocus: vi.fn(),
    ...overrides,
  };

  return {
    props,
    ...render(<TagTextInput {...props} />),
  };
}

describe("TagTextInput", () => {
  it("renders the current value and disabled state", () => {
    renderInput({ value: "AI", disabled: true });

    const input = screen.getByPlaceholderText("输入标签");

    expect(input).toHaveValue("AI");
    expect(input).toBeDisabled();
  });

  it("forwards input, keyboard and focus events", () => {
    const { props } = renderInput();
    const input = screen.getByPlaceholderText("输入标签");

    fireEvent.change(input, { target: { value: "科技" } });
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.focus(input);
    fireEvent.blur(input);

    expect(props.onChangeValue).toHaveBeenCalledWith("科技");
    expect(props.onKeyDown).toHaveBeenCalled();
    expect(props.onFocus).toHaveBeenCalled();
    expect(props.onBlur).toHaveBeenCalled();
  });
});
