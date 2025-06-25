import React from 'react';
import clsx from 'clsx';

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
}

export const Input: React.FC<InputProps> = ({
  label,
  className,
  ...props
}) => {
  return (
    <div>
      {label && (
        <label className="text-gray-400 text-sm">{label}</label>
      )}
      <input
        className={clsx(
          'w-full mt-1 bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600',
          'outline-none focus:border-purple-500 transition-colors',
          className
        )}
        {...props}
      />
    </div>
  );
};