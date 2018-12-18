import React from 'react'
import { sortBy } from 'lodash';
import { storiesOf } from '@storybook/react'
import match from './../utils/match';
import Box from './../Box';
import {
  Table,
  Column,
  TextCell,
  SortHeaderCell,
  SortTypes,
  EmptyIndicator
} from './../DataTable';

storiesOf('DataTable', module)
  .add('Basic', () => (
    <TableSample data={data}/>
  ))
  .add('Empty Table', () => (
    <TableSample data={[]}/>
  ))
  .add('Nothing found', () => (
    <TableSample data={data} filter="no_results"/>
  ))

class TableSample extends React.Component {

  searchableProps = ['addr', 'hostname', 'tags'];

  constructor(props) {
    super(props);
    this.state = {
      filter: props.filter || '',
      colSortDirs: {
        hostname: SortTypes.DESC
      }
    };
  }

  onSortChange = (columnKey, sortDir) => {
    this.state.colSortDirs = { [columnKey]: sortDir };
    this.setState(this.state);
  }

  onFilterChange = value => {
    this.state.filter = value;
    this.setState(this.state);
  }

  searchAndFilterCb(targetValue, searchValue, propName){
    if(propName === 'tags'){
      return targetValue.some((item) => {
        const { name, value } = item;
        return name.toLocaleUpperCase().indexOf(searchValue) !==-1 ||
          value.toLocaleUpperCase().indexOf(searchValue) !==-1;
      });
    }
  }

  sortAndFilter(data) {
    const { colSortDirs } = this.state;
    const filtered = data
      .filter(obj => match(obj, this.state.filter, {
        searchableProps: this.searchableProps,
        cb: this.searchAndFilterCb
      }));

    const columnKey = Object.getOwnPropertyNames(colSortDirs)[0];
    const sortDir = colSortDirs[columnKey];
    let sorted = sortBy(filtered, columnKey);
    if(sortDir === SortTypes.ASC){
      sorted = sorted.reverse();
    }

    return sorted;
  }

  render() {
    let { data } = this.props;
    data = this.sortAndFilter(data);
    const nothingFound = data.length === 0 && this.state.filter.length > 0;

    if (nothingFound) {
      return <EmptyIndicator text="No matching nodes found" />;
    }

    return (
      <Box m={3}>
        <Table rowCount={data.length}>
          <Column
            columnKey="hostname"
            header={
              <SortHeaderCell
                sortDir={this.state.colSortDirs.hostname}
                onSortChange={this.onSortChange}
                title="Hostname"
              />
            }
            cell={<TextCell data={data}/> }
          />
          <Column
            columnKey="addr"
            header={
              <SortHeaderCell
                sortDir={this.state.colSortDirs.addr}
                onSortChange={this.onSortChange}
                title="Address"
              />
            }
            cell={<TextCell data={data}/> }
          />
        </Table>
      </Box>
    )
  }
}

const data = [{
  hostname: 'host-a',
  addr: '192.168.7.1'
},
{
  hostname: 'host-b',
  addr: '192.168.7.2'
},
{
  hostname: 'host-c',
  addr: '192.168.7.3'
}]