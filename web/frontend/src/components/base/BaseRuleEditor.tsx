import React, { useState, useEffect } from 'react';
import { Modal, Form, Button, Space, message } from 'antd';
import { Rule, Condition, Action } from '../../types/rule';

export interface BaseRuleEditorProps {
  visible: boolean;
  onClose: () => void;
  onSave: (rule: Rule) => Promise<void>;
  rule?: Rule;
  title: string;
  dataTypeName: string;
}

export interface BaseRuleEditorState {
  saving: boolean;
  validationErrors: string[];
}

/**
 * 基础规则编辑器 - 所有专门化编辑器的父类
 * 提供公共的状态管理、表单处理、验证和保存逻辑
 */
export abstract class BaseRuleEditor<P extends BaseRuleEditorProps = BaseRuleEditorProps> extends React.Component<P, BaseRuleEditorState> {
  protected form = Form.useForm ? Form.useForm()[0] : null;
  
  constructor(props: P) {
    super(props);
    this.state = {
      saving: false,
      validationErrors: []
    };
  }

  componentDidUpdate(prevProps: P) {
    if (this.props.visible && !prevProps.visible && this.props.rule) {
      this.loadRuleData(this.props.rule);
    }
  }

  /**
   * 加载规则数据到表单 - 子类可重写以自定义加载逻辑
   */
  protected loadRuleData(rule: Rule) {
    if (this.form) {
      this.form.setFieldsValue({
        name: rule.name || '',
        description: rule.description || '',
        priority: rule.priority || 50,
        enabled: rule.enabled !== false
      });
    }
    this.loadConditions(rule.conditions);
    this.loadActions(rule.actions || []);
  }

  /**
   * 加载条件数据 - 子类必须实现
   */
  protected abstract loadConditions(conditions?: Condition): void;

  /**
   * 加载动作数据 - 子类必须实现
   */
  protected abstract loadActions(actions: Action[]): void;

  /**
   * 构建条件对象 - 子类必须实现
   */
  protected abstract buildConditions(): Condition | undefined;

  /**
   * 构建动作数组 - 子类必须实现
   */
  protected abstract buildActions(): Action[];

  /**
   * 验证规则数据 - 子类可重写以添加自定义验证
   */
  protected validateRule(rule: Rule): string[] {
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
  }

  /**
   * 处理保存操作
   */
  protected handleSave = async () => {
    try {
      this.setState({ saving: true, validationErrors: [] });
      
      // 获取表单基础数据
      const formData = this.form ? await this.form.validateFields() : {};
      
      // 构建完整的规则对象
      const rule: Rule = {
        id: this.props.rule?.id || '',
        name: formData.name || '',
        description: formData.description || '',
        priority: formData.priority || 50,
        enabled: formData.enabled !== false,
        data_type: this.props.rule?.data_type,
        conditions: this.buildConditions(),
        actions: this.buildActions(),
        tags: this.props.rule?.tags || {},
        version: this.props.rule?.version || 1,
        created_at: this.props.rule?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      };
      
      // 验证规则
      const errors = this.validateRule(rule);
      if (errors.length > 0) {
        this.setState({ validationErrors: errors });
        message.error(`验证失败: ${errors.join(', ')}`);
        return;
      }
      
      // 调用父组件保存方法
      await this.props.onSave(rule);
      message.success('规则保存成功');
      this.props.onClose();
      
    } catch (error: any) {
      console.error('保存规则失败:', error);
      message.error('保存规则失败：' + (error.message || '未知错误'));
    } finally {
      this.setState({ saving: false });
    }
  };

  /**
   * 处理取消操作
   */
  protected handleCancel = () => {
    if (this.form) {
      this.form.resetFields();
    }
    this.setState({ validationErrors: [] });
    this.props.onClose();
  };

  /**
   * 渲染模态框底部按钮
   */
  protected renderFooter() {
    return (
      <Space>
        <Button onClick={this.handleCancel}>
          取消
        </Button>
        <Button 
          type="primary" 
          loading={this.state.saving}
          onClick={this.handleSave}
        >
          保存规则
        </Button>
      </Space>
    );
  }

  /**
   * 渲染基础表单字段
   */
  protected renderBaseForm() {
    return (
      <Form 
        form={this.form}
        layout="vertical"
        initialValues={{
          priority: 50,
          enabled: true
        }}
      >
        <Form.Item
          label="规则名称"
          name="name"
          rules={[{ required: true, message: '请输入规则名称' }]}
        >
          <input placeholder="请输入规则名称" />
        </Form.Item>
        
        <Form.Item
          label="规则描述"
          name="description"
        >
          <textarea 
            placeholder="请输入规则描述" 
            rows={2}
          />
        </Form.Item>
        
        <Form.Item
          label="优先级"
          name="priority"
          rules={[
            { required: true, message: '请输入优先级' },
            { type: 'number', min: 0, max: 100, message: '优先级必须在0-100之间' }
          ]}
        >
          <input type="number" min={0} max={100} />
        </Form.Item>
        
        <Form.Item
          label="启用状态"
          name="enabled"
          valuePropName="checked"
        >
          <input type="checkbox" />
        </Form.Item>
      </Form>
    );
  }

  /**
   * 渲染专门化内容 - 子类必须实现
   */
  protected abstract renderSpecializedContent(): React.ReactNode;

  /**
   * 主渲染方法
   */
  render() {
    return (
      <Modal
        title={`${this.props.title} - ${this.props.dataTypeName}`}
        open={this.props.visible}
        onCancel={this.handleCancel}
        footer={this.renderFooter()}
        width={800}
        destroyOnHidden
      >
        <div>
          {this.renderBaseForm()}
          {this.state.validationErrors.length > 0 && (
            <div style={{ marginBottom: 16, color: '#ff4d4f' }}>
              <strong>验证错误：</strong>
              <ul>
                {this.state.validationErrors.map((error, index) => (
                  <li key={index}>{error}</li>
                ))}
              </ul>
            </div>
          )}
          {this.renderSpecializedContent()}
        </div>
      </Modal>
    );
  }
}

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
      
    } catch (error: any) {
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

export default BaseRuleEditor;