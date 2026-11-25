import { useState, useEffect } from 'react';
import { Form, message } from 'antd';
import type { Rule, Condition, Action } from '../../types/rule';
import type { BaseRuleEditorProps } from './BaseRuleEditor';
import { asHttpError } from '../../utils/errors';

/**
 * 基础规则编辑器Hook版本 - 用于函数式组件
 */
export const useBaseRuleEditor = (props: BaseRuleEditorProps) => {
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const [validationErrors, setValidationErrors] = useState<string[]>([]);

  useEffect(() => {
    if (props.visible && props.rule) {
      form.setFieldsValue({
        name: props.rule.name || '',
        description: props.rule.description || '',
        priority: props.rule.priority || 50,
        enabled: props.rule.enabled !== false
      });
    }
  }, [props.visible, props.rule, form]);

  const validateRule = (rule: Rule): string[] => {
    const errors: string[] = [];
    
    if (!rule.name || rule.name.trim() === '') {
      errors.push('规则名称不能为空');
    }
    
    if (!rule.conditions) {
      errors.push('必须设置至少一个条件');
    }
    
    if (!rule.actions || rule.actions.length === 0) {
      errors.push('必须设置至少一个动作');
    }
    
    if (typeof rule.priority !== 'number' || rule.priority < 0 || rule.priority > 100) {
      errors.push('优先级必须是0-100之间的数字');
    }
    
    return errors;
  };

  const handleSave = async (buildConditions: () => Condition | undefined, buildActions: () => Action[]) => {
    try {
      setSaving(true);
      setValidationErrors([]);
      
      const formData = await form.validateFields();
      
      const rule: Rule = {
        id: props.rule?.id || '',
        name: formData.name || '',
        description: formData.description || '',
        priority: formData.priority || 50,
        enabled: formData.enabled !== false,
        data_type: props.rule?.data_type,
        conditions: buildConditions(),
        actions: buildActions(),
        tags: props.rule?.tags || {},
        version: props.rule?.version || 1,
        created_at: props.rule?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      };
      
      const errors = validateRule(rule);
      if (errors.length > 0) {
        setValidationErrors(errors);
        message.error(`验证失败: ${errors.join(', ')}`);
        return;
      }
      
      await props.onSave(rule);
      message.success('规则保存成功');
      props.onClose();
      
    } catch (caught) {
      const error = asHttpError(caught);
      console.error('保存规则失败:', error);
      message.error('保存规则失败：' + (error.message || '未知错误'));
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    form.resetFields();
    setValidationErrors([]);
    props.onClose();
  };

  return {
    form,
    saving,
    validationErrors,
    handleSave,
    handleCancel
  };
};
