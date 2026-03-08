"use client";

import * as React from "react";
import {
  Controller,
  FormProvider,
  useFormContext,
  type ControllerProps,
  type FieldPath,
  type FieldValues,
} from "react-hook-form";

const Form = FormProvider;

type FormFieldContextValue<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> = {
  name: TName;
};

const FormFieldContext = React.createContext<FormFieldContextValue>(
  {} as FormFieldContextValue,
);

function FormField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({ ...props }: ControllerProps<TFieldValues, TName>) {
  return (
    <FormFieldContext.Provider value={{ name: props.name }}>
      <Controller {...props} />
    </FormFieldContext.Provider>
  );
}

function useFormField() {
  const fieldContext = React.useContext(FormFieldContext);
  const { getFieldState, formState } = useFormContext();
  const fieldState = getFieldState(fieldContext.name, formState);

  return {
    name: fieldContext.name,
    error: fieldState.error,
  };
}

function FormItem({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={className} {...props} />;
}

function FormLabel(props: React.ComponentProps<"label">) {
  return <label {...props} />;
}

function FormControl(props: React.ComponentProps<"div">) {
  return <div {...props} />;
}

function FormDescription(props: React.ComponentProps<"p">) {
  return <p {...props} />;
}

function FormMessage(props: React.ComponentProps<"p">) {
  const { error } = useFormField();

  if (!error?.message) {
    return null;
  }

  return <p {...props}>{String(error.message)}</p>;
}

export {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  useFormField,
};
