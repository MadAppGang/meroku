import React from 'react';
import clsx from 'clsx';

export interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  options?: Array<{ value: string; label: string }>;
}

export const Select: React.FC<SelectProps> = ({
  label,
  options,
  children,
  className,
  ...props
}) => {
  return (
    <div>
      {label && (
        <label className="text-gray-400 text-sm">{label}</label>
      )}
      <select
        className={clsx(
          'w-full mt-1 bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600',
          'outline-none focus:border-purple-500 transition-colors',
          className
        )}
        {...props}
      >
        {options
          ? options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))
          : children}
      </select>
    </div>
  );
};