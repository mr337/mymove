import React from 'react';
import { Admin, Resource } from 'react-admin';
import restProvider from 'ra-data-simple-rest';
import { history } from 'shared/store';
import customRoutes from './customRoutes';
import { ConnectedRouter } from 'react-router-redux';
import { Switch, Route } from 'react-router-dom';
import { Landing } from './Landing';

const dataProvider = restProvider('http://admin/v1/...');

const AdminWrapper = () => (
  <ConnectedRouter history={history}>
    <Switch>
      <Route exact path="/" component={Landing} />
      <Admin customRoutes={customRoutes} dataProvider={dataProvider} history={history}>
        <Resource />
      </Admin>
    </Switch>
  </ConnectedRouter>
);

export default AdminWrapper;
